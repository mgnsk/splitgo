// Package server glues the webkit framework to the splitgo views and models.
package server

import (
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/webkit/web"
)

// UserKey is the request context key of the signed-in user.
var UserKey web.ContextKey[domain.User]
