// Copyright 2026 Canonical.

//go:generate go tool mockgen -package idpgroupfetcher -typed -destination ./mocks_test.go github.com/canonical/hook-service/gen/hook/groups/v1 GroupsMappingServiceClient
//go:generate go tool mockgen -package idpgroupfetcher -typed -destination ./mocks_stream_test.go -mock_names GroupsMappingService_GetGroupsForUserClient=MockGroupStream github.com/canonical/hook-service/gen/hook/groups/v1 GroupsMappingService_GetGroupsForUserClient

package idpgroupfetcher
