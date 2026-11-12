package counter

import "net/http"

type Counter interface {
	http.Handler
	Close() error // saves the latest state; called on shutdown
}
