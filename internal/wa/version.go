package wa

import (
	"context"
	"net/http"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// whatsmeow ships a hard-coded WhatsApp Web version that the servers
// eventually reject (405 "client outdated") as the library ages. Before
// connecting we ask web.whatsapp.com for the current revision and advertise
// that instead when it is newer.
var (
	versionMu      sync.Mutex
	versionUpdated bool
)

// refreshWAVersion updates the advertised client version once per process
// (again when force is set, e.g. after an "outdated" rejection). It must run
// before connecting, since whatsmeow reads these globals without locking.
func refreshWAVersion(ctx context.Context, hc *http.Client, log waLog.Logger, force bool) {
	versionMu.Lock()
	defer versionMu.Unlock()
	if versionUpdated && !force {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	latest, err := whatsmeow.GetLatestVersion(ctx, hc)
	if err != nil {
		log.Warnf("couldn't fetch the current WhatsApp Web version (keeping %s): %v", store.GetWAVersion(), err)
		return
	}
	versionUpdated = true
	if cur := store.GetWAVersion(); !cur.LessThan(*latest) {
		return
	}
	store.SetWAVersion(*latest)
	log.Infof("Using WhatsApp Web version %s", latest)
}
