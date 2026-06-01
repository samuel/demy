package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/samuel/demy/internal/model"
)

const (
	sessionTTL      = 30 * 24 * time.Hour
	sessionTouchGap = 5 * time.Minute
)

// webAuthnUser adapts model.User and its passkeys to the webauthn.User interface.
type webAuthnUser struct {
	id          []byte
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte                         { return u.id }
func (u *webAuthnUser) WebAuthnName() string                       { return u.name }
func (u *webAuthnUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// toWebAuthnUser builds a webAuthnUser from a model.User and its stored passkeys.
func toWebAuthnUser(u *model.User, passkeys []*model.PasskeyCredential) *webAuthnUser {
	wu := &webAuthnUser{
		id:          []byte(u.ID),
		name:        u.Email,
		displayName: u.DisplayName,
		credentials: make([]webauthn.Credential, 0, len(passkeys)),
	}
	for _, pk := range passkeys {
		transports := make([]protocol.AuthenticatorTransport, 0, len(pk.Transports))
		for _, t := range pk.Transports {
			transports = append(transports, protocol.AuthenticatorTransport(t))
		}
		wu.credentials = append(wu.credentials, webauthn.Credential{
			ID:        pk.CredentialID,
			PublicKey: pk.PublicKey,
			Authenticator: webauthn.Authenticator{
				AAGUID:    pk.AAGUID,
				SignCount: pk.SignCount,
			},
			Transport: transports,
		})
	}
	return wu
}

// RegistrationBegin starts the WebAuthn registration flow.
// Returns creation options JSON bytes and a session challenge to store temporarily.
func (s *Service) RegistrationBegin(ctx context.Context, displayName, email string) (optionsJSON []byte, sessionData []byte, err error) {
	// Build a temporary user stub; ID is provisional until RegistrationFinish.
	wu := &webAuthnUser{
		id:          []byte(uuid.New().String()),
		name:        email,
		displayName: displayName,
	}

	creation, session, err := s.wauth.BeginRegistration(wu)
	if err != nil {
		return nil, nil, fmt.Errorf("begin registration: %w", err)
	}

	optionsJSON, err = json.Marshal(creation)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal creation options: %w", err)
	}

	sessionData, err = json.Marshal(session)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal session data: %w", err)
	}

	return optionsJSON, sessionData, nil
}

// RegistrationFinish completes the WebAuthn registration.
// sessionData is what was returned by RegistrationBegin.
// Returns the new user (with ID assigned) and creates the passkey credential in the store.
func (s *Service) RegistrationFinish(ctx context.Context, displayName, email string, sessionData []byte, responseJSON []byte) (*model.User, error) {
	var session webauthn.SessionData
	if err := json.Unmarshal(sessionData, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session data: %w", err)
	}

	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(responseJSON))
	if err != nil {
		return nil, fmt.Errorf("parse credential creation response: %w", err)
	}

	// Build a matching stub so CreateCredential can verify against the session's user handle.
	wu := &webAuthnUser{
		id:          session.UserID,
		name:        email,
		displayName: displayName,
	}

	credential, err := s.wauth.CreateCredential(wu, session, parsedResponse)
	if err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}

	// Determine whether this is the first user; first is admin.
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}

	now := time.Now().UTC()
	user := &model.User{
		ID:          uuid.New().String(),
		DisplayName: displayName,
		Email:       email,
		IsAdmin:     count == 0,
		CreatedAt:   now,
	}

	if err := s.store.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	transports := make([]string, 0, len(credential.Transport))
	for _, t := range credential.Transport {
		transports = append(transports, string(t))
	}

	pk := &model.PasskeyCredential{
		ID:           uuid.New().String(),
		UserID:       user.ID,
		CredentialID: credential.ID,
		PublicKey:    credential.PublicKey,
		AAGUID:       credential.Authenticator.AAGUID,
		SignCount:    credential.Authenticator.SignCount,
		Transports:   transports,
		CreatedAt:    now,
		LastUsedAt:   now,
	}

	if err := s.store.CreatePasskey(ctx, pk); err != nil {
		return nil, fmt.Errorf("create passkey: %w", err)
	}

	return user, nil
}

// LoginBegin starts the WebAuthn authentication flow.
// email may be empty for discoverable-credential flow.
func (s *Service) LoginBegin(ctx context.Context, email string) (optionsJSON []byte, sessionData []byte, err error) {
	var assertion *protocol.CredentialAssertion
	var session *webauthn.SessionData

	if email == "" {
		// Discoverable (resident key) flow — no user hint.
		assertion, session, err = s.wauth.BeginDiscoverableLogin()
	} else {
		u, fetchErr := s.store.GetUserByEmail(ctx, email)
		if fetchErr != nil {
			return nil, nil, fmt.Errorf("get user: %w", fetchErr)
		}
		passkeys, fetchErr := s.store.GetPasskeysByUserID(ctx, u.ID)
		if fetchErr != nil {
			return nil, nil, fmt.Errorf("get passkeys: %w", fetchErr)
		}
		wu := toWebAuthnUser(u, passkeys)
		assertion, session, err = s.wauth.BeginLogin(wu)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("begin login: %w", err)
	}

	optionsJSON, err = json.Marshal(assertion)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal assertion options: %w", err)
	}

	sessionData, err = json.Marshal(session)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal session data: %w", err)
	}

	return optionsJSON, sessionData, nil
}

// LoginFinish completes the WebAuthn authentication.
func (s *Service) LoginFinish(ctx context.Context, sessionData []byte, responseJSON []byte) (*model.User, error) {
	var session webauthn.SessionData
	if err := json.Unmarshal(sessionData, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session data: %w", err)
	}

	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(responseJSON))
	if err != nil {
		return nil, fmt.Errorf("parse credential request response: %w", err)
	}

	var credential *webauthn.Credential
	var userID string

	if len(session.UserID) == 0 {
		// Discoverable login: look up user from the credential's user handle.
		credential, err = s.wauth.ValidateDiscoverableLogin(
			func(rawID, userHandle []byte) (webauthn.User, error) {
				uid := string(userHandle)
				u, uerr := s.store.GetUserByID(ctx, uid)
				if uerr != nil {
					return nil, fmt.Errorf("get user: %w", uerr)
				}
				passkeys, perr := s.store.GetPasskeysByUserID(ctx, u.ID)
				if perr != nil {
					return nil, fmt.Errorf("get passkeys: %w", perr)
				}
				return toWebAuthnUser(u, passkeys), nil
			},
			session,
			parsedResponse,
		)
		if err != nil {
			return nil, fmt.Errorf("validate discoverable login: %w", err)
		}
		userID = string(parsedResponse.Response.UserHandle)
	} else {
		// Non-discoverable: user is known from the session.
		uid := string(session.UserID)
		u, uerr := s.store.GetUserByID(ctx, uid)
		if uerr != nil {
			return nil, fmt.Errorf("get user: %w", uerr)
		}
		passkeys, perr := s.store.GetPasskeysByUserID(ctx, u.ID)
		if perr != nil {
			return nil, fmt.Errorf("get passkeys: %w", perr)
		}
		wu := toWebAuthnUser(u, passkeys)
		credential, err = s.wauth.ValidateLogin(wu, session, parsedResponse)
		if err != nil {
			return nil, fmt.Errorf("validate login: %w", err)
		}
		userID = uid
	}

	// Update sign count on the stored passkey.
	pk, err := s.store.GetPasskeyByCredentialID(ctx, credential.ID)
	if err != nil {
		return nil, fmt.Errorf("get passkey by credential id: %w", err)
	}
	pk.SignCount = credential.Authenticator.SignCount
	pk.LastUsedAt = time.Now().UTC()
	if err := s.store.UpdatePasskey(ctx, pk); err != nil {
		return nil, fmt.Errorf("update passkey: %w", err)
	}

	u, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

// CreateSession creates a new session for a user.
func (s *Service) CreateSession(ctx context.Context, userID string) (*model.Session, error) {
	var tokenBytes [32]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	now := time.Now().UTC()
	sess := &model.Session{
		ID:         hex.EncodeToString(tokenBytes[:]),
		UserID:     userID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(sessionTTL),
		LastUsedAt: now,
	}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return sess, nil
}

// GetSession retrieves a valid (non-expired) session and touches it if stale.
func (s *Service) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	now := time.Now().UTC()
	if now.After(sess.ExpiresAt) {
		return nil, ErrUnauthorized
	}
	if now.Sub(sess.LastUsedAt) > sessionTouchGap {
		if err := s.store.TouchSession(ctx, sess.ID, now); err != nil {
			return nil, fmt.Errorf("touch session: %w", err)
		}
		sess.LastUsedAt = now
		sess.ExpiresAt = now.Add(sessionTTL)
	}
	return sess, nil
}

// DeleteSession removes a session.
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	if err := s.store.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// GetUser fetches a user by ID.
func (s *Service) GetUser(ctx context.Context, id string) (*model.User, error) {
	u, err := s.store.GetUserByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

// UpdateUser saves changes to a user record.
func (s *Service) UpdateUser(ctx context.Context, u *model.User) error {
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// GetPasskeys returns all passkeys for a user.
func (s *Service) GetPasskeys(ctx context.Context, userID string) ([]*model.PasskeyCredential, error) {
	pks, err := s.store.GetPasskeysByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get passkeys: %w", err)
	}
	return pks, nil
}

// DeletePasskey removes a passkey by ID after verifying it belongs to userID.
func (s *Service) DeletePasskey(ctx context.Context, userID, id string) error {
	pks, err := s.store.GetPasskeysByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get passkeys: %w", err)
	}
	var found bool
	for _, pk := range pks {
		if pk.ID == id {
			found = true
			break
		}
	}
	if !found {
		return ErrNotFound
	}
	if err := s.store.DeletePasskey(ctx, id); err != nil {
		return fmt.Errorf("delete passkey: %w", err)
	}
	return nil
}

// ListUsers returns all users (admin-only in the web layer).
func (s *Service) ListUsers(ctx context.Context) ([]*model.User, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// UpdateUserAdmin sets the is_admin flag on a user.
func (s *Service) UpdateUserAdmin(ctx context.Context, targetUserID string, isAdmin bool) error {
	u, err := s.store.GetUserByID(ctx, targetUserID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	u.IsAdmin = isAdmin
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}
