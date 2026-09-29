package plugin

import (
	"fmt"
	"sync"
	"testing"
)

// The hourly cache cleanup runs while queries read and save WebIDs: it must hold the lock the queries use, or Go
// stops the plugin with "concurrent map writes" (run with -race to detect it).
func TestCleanWebIDCacheConcurrentWithQueries(t *testing.T) {
	d := newTestDatasource()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				path := fmt.Sprintf(`\\AF\DB\E%d|A%d`, w, i%50)
				d.saveWebID(map[string]interface{}{"WebId": "W" + path, "Type": "Double"}, path, false)
				d.getWebIDEntry("W" + path)
			}
		}(w)
	}
	for i := 0; i < 200; i++ {
		d.cleanWebIDCache()
	}
	close(stop)
	wg.Wait()
}
