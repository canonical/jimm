// Copyright 2026 Canonical.

package idpgroupfetcher

import (
	"context"
	"errors"
	"io"
	"testing"

	qt "github.com/frankban/quicktest"
	"go.uber.org/mock/gomock"

	groupspb "github.com/canonical/hook-service/gen/hook/groups/v1"
)

const testHookServiceAddress = "localhost:9091"

// newMockHookService returns a HookService whose gRPC client and stream
// are gomock mocks, plus their recorders for scripting calls.
func newMockHookService(c *qt.C) (*HookService, *MockGroupsMappingServiceClientMockRecorder, *MockGroupStreamMockRecorder[groupspb.GroupMapping]) {
	ctrl := gomock.NewController(c)
	client := NewMockGroupsMappingServiceClient(ctrl)
	stream := NewMockGroupStream[groupspb.GroupMapping](ctrl)
	return &HookService{client: client, token: "dummy-token"}, client.EXPECT(), stream.EXPECT()
}

func TestNewHookServiceReturnsErrorForInvalidAddress(t *testing.T) {
	c := qt.New(t)
	// "%" is an invalid URL escape, so grpc.NewClient fails while
	// parsing the target.
	_, err := NewHookService("%", "dummy-token")
	c.Assert(err, qt.ErrorMatches, `failed to create hook-service client: .*invalid.*`)
}

func TestHookServiceFetchGroupsGetGroupsError(t *testing.T) {
	c := qt.New(t)
	fetcher, client, _ := newMockHookService(c)
	client.GetGroupsForUser(gomock.Any(), gomock.Any()).Return(nil, errors.New("get groups failed"))

	_, err := fetcher.FetchGroups(context.Background(), "alice@example.com")
	c.Assert(err, qt.ErrorMatches, `failed to fetch groups for user "alice@example.com": get groups failed`)
}

func TestHookServiceFetchGroupsStreamError(t *testing.T) {
	c := qt.New(t)
	fetcher, client, stream := newMockHookService(c)
	client.GetGroupsForUser(gomock.Any(), gomock.Any()).Return(stream.mock, nil)
	stream.Recv().Return(nil, errors.New("stream failed"))

	_, err := fetcher.FetchGroups(context.Background(), "alice@example.com")
	c.Assert(err, qt.ErrorMatches, `failed to fetch groups for user "alice@example.com": stream failed`)
}

func TestHookServiceFetchGroupsSkipsEmptyNames(t *testing.T) {
	c := qt.New(t)
	fetcher, client, stream := newMockHookService(c)
	client.GetGroupsForUser(gomock.Any(), gomock.Any()).Return(stream.mock, nil)
	stream.Recv().Return(&groupspb.GroupMapping{}, nil)
	stream.Recv().Return(&groupspb.GroupMapping{Name: "canonical"}, nil)
	stream.Recv().Return(nil, io.EOF)

	groups, err := fetcher.FetchGroups(context.Background(), "alice@example.com")
	c.Assert(err, qt.IsNil)
	c.Assert(groups, qt.DeepEquals, []string{"canonical"})
}

// TestNewWithoutFetcher verifies the factory returns a NoOp fetcher that
// resolves no groups when the type is explicitly set to none.
func TestNewWithoutFetcher(t *testing.T) {
	c := qt.New(t)

	fetcher, err := New(Params{Type: TypeNone})
	c.Assert(err, qt.IsNil)
	c.Assert(fetcher != nil, qt.IsTrue)

	groups, err := fetcher.FetchGroups(context.Background(), "alice@example.com")
	c.Assert(err, qt.IsNil)
	c.Assert(groups, qt.HasLen, 0)
}

// TestNewUnknownType verifies the factory fails for an unknown fetcher type.
func TestNewUnknownType(t *testing.T) {
	c := qt.New(t)

	_, err := New(Params{Type: Type("unknown")})
	c.Assert(err, qt.ErrorMatches, `unknown idp group fetcher type "unknown"`)
}

// TestNewHookServiceType verifies the factory builds a hook-service
// fetcher for the hook-service type.
func TestNewHookServiceType(t *testing.T) {
	c := qt.New(t)

	fetcher, err := New(Params{Type: TypeHookService, HookServiceAddress: testHookServiceAddress, HookServiceToken: "dummy-token"})
	c.Assert(err, qt.IsNil)
	c.Assert(fetcher != nil, qt.IsTrue)
}
