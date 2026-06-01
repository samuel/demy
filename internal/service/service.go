package service

import (
	"fmt"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/samuel/demy/internal/model"
)

// Service is the application business logic layer.
type Service struct {
	store  model.Store
	wauth  *webauthn.WebAuthn
	rpID   string
	rpName string
}

// New creates a Service and initialises the WebAuthn instance.
func New(store model.Store, rpID, rpDisplayName string) (*Service, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: rpDisplayName,
		RPID:          rpID,
		RPOrigins:     []string{"https://" + rpID, "http://" + rpID},
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn init: %w", err)
	}
	return &Service{
		store:  store,
		wauth:  wa,
		rpID:   rpID,
		rpName: rpDisplayName,
	}, nil
}
