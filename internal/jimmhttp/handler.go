package jimmhttp

import (
	"github.com/go-chi/chi/v5"
)

// JIMMHttpHandler represents a http handler for the JIMM service.
type JIMMHttpHandler interface {
	Routes() chi.Router
	SetupMiddleware()
}
