// Copyright 2026 Canonical.

package idpgroupfetcher

import (
	"context"
	"fmt"
	"io"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	groupspb "github.com/canonical/hook-service/gen/hook/groups/v1"
)

// serviceAccountSuffix is appended to service account usernames by the
// identity platform. The hook-service keys service accounts by their raw
// client ID, so the suffix must be stripped before lookup.
const serviceAccountSuffix = "@serviceaccount"

// HookService resolves IdP groups from the Canonical identity platform's
// hook-service via its GroupsMappingService gRPC API.
type HookService struct {
	client groupspb.GroupsMappingServiceClient
	// tokenSource provides the access token sent as a Bearer token on
	// every call. The hook-service verifies it as a JWT issued by the
	// identity provider for JIMM's own OAuth client.
	tokenSource oauth2.TokenSource
}

// NewHookService returns a HookService fetcher connected to the
// hook-service gRPC API at the given address, authenticating with access
// tokens from the given token source.
func NewHookService(address string, tokenSource oauth2.TokenSource) (*HookService, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create hook-service client: %w", err)
	}
	return &HookService{
		client:      groupspb.NewGroupsMappingServiceClient(conn),
		tokenSource: tokenSource,
	}, nil
}

// FetchGroups implements offer.IdPGroupFetcher. It fails closed: any
// error from the hook-service denies access.
func (h *HookService) FetchGroups(ctx context.Context, username string) ([]string, error) {
	userID := strings.TrimSuffix(username, serviceAccountSuffix)

	token, err := h.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain hook-service access token: %w", err)
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token.AccessToken)
	stream, err := h.client.GetGroupsForUser(ctx, &groupspb.GetGroupsForUserReq{
		UserId: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch groups for user %q: %w", username, err)
	}

	var groupNames []string
	for {
		group, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to fetch groups for user %q: %w", username, err)
		}
		if name := group.GetName(); name != "" {
			groupNames = append(groupNames, name)
		}
	}
	return groupNames, nil
}
