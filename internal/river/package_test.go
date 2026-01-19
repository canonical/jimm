package river

var NewUpgradeToWorker = newUpgradeToWorker

//go:generate go tool mockgen -package river_test -typed -destination ./river_mock_test.go github.com/canonical/jimm/v3/internal/river UpgradeToManager
