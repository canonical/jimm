// Copyright 2026 Canonical.

package idpgroupfetcher

import (
	"context"
	"fmt"
	"io"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "github.com/canonical/hook-service/gen/hook/groups/v1"
)

// serviceAccountSuffix is appended to service account usernames by the
// identity platform. The hook-service keys service accounts by their raw
// client ID, so the suffix must be stripped before lookup.
const serviceAccountSuffix = "@serviceaccount"

// HookService resolves IdP groups from the Canonical identity platform's
// hook-service via its GroupsMappingService gRPC API.
type HookService struct {
	client pb.GroupsMappingServiceClient
	// token is sent as a Bearer token on every call; the hook-service
	// gRPC interceptor requires the header even when authentication is
	// disabled.
	token string
}

// NewHookService returns a HookService fetcher connected to the
// hook-service gRPC API at the given address.
func NewHookService(address, token string) (*HookService, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create hook-service client: %w", err)
	}
	return &HookService{
		client: pb.NewGroupsMappingServiceClient(conn),
		token:  token,
	}, nil
}

// FetchGroups implements offer.IdPGroupFetcher. It fails closed: any
// error from the hook-service denies access.
func (h *HookService) FetchGroups(ctx context.Context, username string) ([]string, error) {
	userID := strings.TrimSuffix(username, serviceAccountSuffix)

	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+h.token)
	stream, err := h.client.GetGroupsForUser(ctx, &pb.GetGroupsForUserReq{
		UserId: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch groups for user %q: %w", username, err)
	}

	var groups []string
	for {
		mapping, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to fetch groups for user %q: %w", username, err)
		}
		if name := mapping.GetName(); name != "" {
			groups = append(groups, name)
		}
	}
	return groups, nil
}
