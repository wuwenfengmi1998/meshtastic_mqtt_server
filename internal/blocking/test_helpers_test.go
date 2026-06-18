package blocking

import (
	"testing"

	"meshtastic_mqtt_server/internal/store"
	"meshtastic_mqtt_server/internal/store/testutil"
)

// openTestStore 委托到 store/testutil，让本包的测试代码保持简洁。
func openTestStore(t *testing.T) *store.Store {
	return testutil.OpenStore(t)
}
