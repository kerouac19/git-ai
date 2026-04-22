package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	accessTokenTTL             = 3600 * time.Second
	refreshTokenTTL            = 90 * 24 * time.Hour
	deviceCodeExpiresInSeconds = 15 * 60
	deviceCodePollIntervalSecs = 5
)

type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type DeviceCodeInfo struct {
	UserCode  string        `json:"userCode"`
	ExpiresAt int64         `json:"expiresAt"`
	Status    string        `json:"status"`
	Subject   *TokenSubject `json:"subject,omitempty"`
}

type deviceCodeRecord struct {
	DeviceCode   string
	UserCode     string
	Status       string
	ExpiresAt    time.Time
	ApprovedAt   *time.Time
	DeniedAt     *time.Time
	LastPolledAt *time.Time
	SubjectJSON  *string
}

type DeviceFlowService struct {
	Pool      *pgxpool.Pool
	JWTSecret string
}

func (s *DeviceFlowService) StartDeviceFlow(ctx context.Context, baseURL string) (*DeviceCodeResponse, error) {
	if err := s.pruneExpiredDeviceCodes(ctx); err != nil {
		return nil, err
	}

	now := time.Now()
	deviceCode := uuid.New().String()
	userCode := makeUserCode()
	verificationURI := baseURL + "/oauth/device"

	expiresAt := now.Add(time.Duration(deviceCodeExpiresInSeconds) * time.Second)

	_, err := s.Pool.Exec(ctx, `
		INSERT INTO public.oauth_device_codes (
			device_code, user_code, client_id, verification_uri,
			status, created_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		deviceCode, userCode, "git-ai-cli", verificationURI,
		"pending", now, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting device code: %w", err)
	}

	return &DeviceCodeResponse{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURI + "?user_code=" + url.QueryEscape(userCode),
		ExpiresIn:               deviceCodeExpiresInSeconds,
		Interval:                deviceCodePollIntervalSecs,
	}, nil
}

func (s *DeviceFlowService) ExchangeDeviceCode(ctx context.Context, deviceCode string) (map[string]interface{}, error) {
	if err := s.pruneExpiredDeviceCodes(ctx); err != nil {
		return nil, err
	}

	entry, err := s.findDeviceCodeByDeviceCode(ctx, deviceCode)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return oauthError("expired_token", "Device code expired or not found"), nil
	}

	if entry.Status == "denied" {
		return oauthError("access_denied", "Device authorization was denied"), nil
	}

	if entry.Status == "approved" {
		subject := s.parseStoredSubject(entry.SubjectJSON)
		if subject == nil {
			return oauthError("server_error", "Device code approved but no user data found"), nil
		}

		if err := s.deleteDeviceCode(ctx, deviceCode); err != nil {
			return nil, err
		}
		return s.issueTokenResponse(*subject)
	}

	claimed, err := s.claimPollSlot(ctx, deviceCode)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return oauthError("slow_down", "Polling too frequently"), nil
	}
	return oauthError("authorization_pending", "Device authorization is still pending"), nil
}

func (s *DeviceFlowService) ExchangeRefreshToken(refreshToken string) (map[string]interface{}, error) {
	claims, err := VerifyToken(refreshToken, s.JWTSecret)
	if err != nil {
		return oauthError("invalid_grant", "Refresh token is invalid or expired"), nil
	}

	if claims.Type != "refresh" {
		return oauthError("invalid_grant", "Refresh token is invalid"), nil
	}

	subject := subjectFromClaims(claims)
	return s.issueTokenResponse(subject)
}

func (s *DeviceFlowService) ExchangeInstallNonce(installNonce string) (map[string]interface{}, error) {
	if installNonce == "" {
		return oauthError("invalid_request", "install_nonce is required"), nil
	}

	truncated := installNonce
	if len(truncated) > 8 {
		truncated = truncated[:8]
	}

	subject := makeDefaultSubject(&TokenSubject{
		Name:  "Install User",
		Email: fmt.Sprintf("install+%s@git-ai.local", truncated),
	})

	return s.issueTokenResponse(subject)
}

func (s *DeviceFlowService) GetDeviceCodeByUserCode(ctx context.Context, userCode string) (*DeviceCodeInfo, error) {
	if err := s.pruneExpiredDeviceCodes(ctx); err != nil {
		return nil, err
	}

	entry, err := s.findDeviceCodeByUserCode(ctx, userCode)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	subject := s.parseStoredSubject(entry.SubjectJSON)
	return &DeviceCodeInfo{
		UserCode:  entry.UserCode,
		ExpiresAt: entry.ExpiresAt.UnixMilli(),
		Status:    entry.Status,
		Subject:   subject,
	}, nil
}

func (s *DeviceFlowService) ApproveDeviceCode(ctx context.Context, userCode string) (*DeviceCodeInfo, error) {
	if err := s.pruneExpiredDeviceCodes(ctx); err != nil {
		return nil, err
	}

	entry, err := s.findDeviceCodeByUserCode(ctx, userCode)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	subject := s.parseStoredSubject(entry.SubjectJSON)
	status := entry.Status

	if entry.Status == "denied" {
		return &DeviceCodeInfo{
			UserCode:  entry.UserCode,
			ExpiresAt: entry.ExpiresAt.UnixMilli(),
			Status:    entry.Status,
			Subject:   subject,
		}, nil
	}

	if entry.Status != "approved" {
		// Only approve if subject data has been set (via UpdateDeviceCodeSubject)
		if subject == nil {
			return nil, fmt.Errorf("cannot approve device code: user data not yet associated (userCode=%s)", userCode)
		}

		tag, err := s.Pool.Exec(ctx, `
			UPDATE public.oauth_device_codes
			SET status = 'approved', approved_at = now()
			WHERE user_code = $1 AND subject_json IS NOT NULL AND user_id IS NOT NULL`, userCode)
		if err != nil {
			return nil, fmt.Errorf("approving device code: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil, fmt.Errorf("cannot approve device code: user data missing in database (userCode=%s)", userCode)
		}
		status = "approved"
	}

	return &DeviceCodeInfo{
		UserCode:  entry.UserCode,
		ExpiresAt: entry.ExpiresAt.UnixMilli(),
		Status:    status,
		Subject:   subject,
	}, nil
}

func (s *DeviceFlowService) DenyDeviceCode(ctx context.Context, userCode string) (*DeviceCodeInfo, error) {
	if err := s.pruneExpiredDeviceCodes(ctx); err != nil {
		return nil, err
	}

	entry, err := s.findDeviceCodeByUserCode(ctx, userCode)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	subject := s.parseStoredSubject(entry.SubjectJSON)
	status := entry.Status

	if entry.Status != "approved" {
		_, err = s.Pool.Exec(ctx, `
			UPDATE public.oauth_device_codes
			SET status = 'denied', denied_at = now()
			WHERE user_code = $1`, userCode)
		if err != nil {
			return nil, fmt.Errorf("denying device code: %w", err)
		}
		status = "denied"
	}

	return &DeviceCodeInfo{
		UserCode:  entry.UserCode,
		ExpiresAt: entry.ExpiresAt.UnixMilli(),
		Status:    status,
		Subject:   subject,
	}, nil
}

func (s *DeviceFlowService) UpdateDeviceCodeSubject(ctx context.Context, userCode string, subject TokenSubject) error {
	subjectJSON, err := json.Marshal(subject)
	if err != nil {
		return fmt.Errorf("marshaling subject: %w", err)
	}

	tag, err := s.Pool.Exec(ctx, `
		UPDATE public.oauth_device_codes
		SET subject_json = $1::jsonb, user_id = $2
		WHERE user_code = $3 AND status = 'pending'`,
		string(subjectJSON), subject.Sub, userCode)
	if err != nil {
		return fmt.Errorf("updating device code subject: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("device code not updated: not found or no longer pending")
	}
	return nil
}

func (s *DeviceFlowService) IssueBrowserSessionToken(subject TokenSubject) string {
	token, _ := SignAccessToken(subject, s.JWTSecret, accessTokenTTL)
	return token
}

func (s *DeviceFlowService) DecodeAccessToken(accessToken string) (*Claims, error) {
	claims, err := VerifyToken(accessToken, s.JWTSecret)
	if err != nil {
		return nil, err
	}
	if claims.Type != "" && claims.Type != "access" {
		return nil, fmt.Errorf("unexpected token type: %s", claims.Type)
	}
	return claims, nil
}

// --- internal helpers ---

func makeUserCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	letters := hex.EncodeToString(b)[:4]

	n, _ := rand.Int(rand.Reader, big.NewInt(9000))
	digits := n.Int64() + 1000

	return fmt.Sprintf("%s-%04d", strings.ToUpper(letters), digits)
}

func makeDefaultSubject(overrides *TokenSubject) TokenSubject {
	sub := envOrDefault("DEFAULT_USER_ID", "00000000-0000-0000-0000-000000000001")
	email := envOrDefault("DEFAULT_USER_EMAIL", "git-ai@example.local")
	name := envOrDefault("DEFAULT_USER_NAME", "Git AI User")
	personalOrgID := envOrDefault("DEFAULT_PERSONAL_ORG_ID", "git-ai-local-org")
	orgName := envOrDefault("DEFAULT_ORG_NAME", "Git AI Local")
	orgSlug := envOrDefault("DEFAULT_ORG_SLUG", "git-ai-local")
	role := envOrDefault("DEFAULT_USER_ROLE", "user")

	base := TokenSubject{
		Sub:           sub,
		Email:         email,
		Name:          name,
		PersonalOrgID: personalOrgID,
		Orgs: []Org{
			{
				OrgID:   personalOrgID,
				OrgName: orgName,
				OrgSlug: orgSlug,
				Role:    role,
			},
		},
		Role: role,
	}

	if overrides != nil {
		if overrides.Sub != "" {
			base.Sub = overrides.Sub
		}
		if overrides.Email != "" {
			base.Email = overrides.Email
		}
		if overrides.Name != "" {
			base.Name = overrides.Name
		}
		if overrides.PersonalOrgID != "" {
			base.PersonalOrgID = overrides.PersonalOrgID
		}
		if overrides.Role != "" {
			base.Role = overrides.Role
		}
		if overrides.Orgs != nil {
			base.Orgs = overrides.Orgs
		}
	}

	return base
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (s *DeviceFlowService) issueTokenResponse(subject TokenSubject) (map[string]interface{}, error) {
	accessToken, err := SignAccessToken(subject, s.JWTSecret, accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("signing access token: %w", err)
	}

	refreshToken, err := SignRefreshToken(subject, s.JWTSecret, refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("signing refresh token: %w", err)
	}

	return map[string]interface{}{
		"access_token":       accessToken,
		"token_type":         "Bearer",
		"expires_in":         int(accessTokenTTL.Seconds()),
		"refresh_token":      refreshToken,
		"refresh_expires_in": int(refreshTokenTTL.Seconds()),
	}, nil
}

func subjectFromClaims(claims *Claims) TokenSubject {
	defaults := makeDefaultSubject(nil)

	subject := TokenSubject{
		Sub:           claims.Subject,
		Email:         claims.Email,
		Name:          claims.Name,
		PersonalOrgID: claims.PersonalOrgID,
		Orgs:          claims.Orgs,
		Role:          claims.Role,
	}

	if subject.Sub == "" {
		subject.Sub = defaults.Sub
	}
	if subject.Email == "" {
		subject.Email = defaults.Email
	}
	if subject.Name == "" {
		subject.Name = defaults.Name
	}
	if subject.PersonalOrgID == "" {
		subject.PersonalOrgID = defaults.PersonalOrgID
	}
	if subject.Orgs == nil {
		subject.Orgs = defaults.Orgs
	}
	if subject.Role == "" {
		subject.Role = defaults.Role
	}

	return subject
}

func (s *DeviceFlowService) parseStoredSubject(serialized *string) *TokenSubject {
	if serialized == nil || *serialized == "" {
		return nil
	}

	var subject TokenSubject
	if err := json.Unmarshal([]byte(*serialized), &subject); err != nil {
		return nil
	}

	// Only require Sub to be non-empty; Email can be empty for users without email
	if subject.Sub == "" {
		return nil
	}

	return &subject
}

func oauthError(errorCode, description string) map[string]interface{} {
	return map[string]interface{}{
		"error":             errorCode,
		"error_description": description,
	}
}

func (s *DeviceFlowService) pruneExpiredDeviceCodes(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `
		DELETE FROM public.oauth_device_codes
		WHERE expires_at <= now()`)
	return err
}

func (s *DeviceFlowService) findDeviceCodeByDeviceCode(ctx context.Context, deviceCode string) (*deviceCodeRecord, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT device_code, user_code, status, expires_at,
			approved_at, denied_at, last_polled_at,
			CASE WHEN subject_json IS NOT NULL THEN subject_json::text END
		FROM public.oauth_device_codes
		WHERE device_code = $1
		LIMIT 1`, deviceCode)

	return scanDeviceCodeRecord(row)
}

func (s *DeviceFlowService) findDeviceCodeByUserCode(ctx context.Context, userCode string) (*deviceCodeRecord, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT device_code, user_code, status, expires_at,
			approved_at, denied_at, last_polled_at,
			CASE WHEN subject_json IS NOT NULL THEN subject_json::text END
		FROM public.oauth_device_codes
		WHERE user_code = $1
		LIMIT 1`, userCode)

	return scanDeviceCodeRecord(row)
}

func scanDeviceCodeRecord(row pgx.Row) (*deviceCodeRecord, error) {
	var r deviceCodeRecord
	err := row.Scan(
		&r.DeviceCode, &r.UserCode, &r.Status, &r.ExpiresAt,
		&r.ApprovedAt, &r.DeniedAt, &r.LastPolledAt,
		&r.SubjectJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning device code record: %w", err)
	}
	return &r, nil
}

// claimPollSlot atomically advances last_polled_at if the previous attempt is
// older than deviceCodePollIntervalSecs. Returns true when the caller owns the
// slot for this interval; false means the client should be told slow_down.
func (s *DeviceFlowService) claimPollSlot(ctx context.Context, deviceCode string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE public.oauth_device_codes
		SET last_polled_at = now()
		WHERE device_code = $1
		  AND (last_polled_at IS NULL
		       OR last_polled_at < now() - make_interval(secs => $2))
	`, deviceCode, deviceCodePollIntervalSecs)
	if err != nil {
		return false, fmt.Errorf("claiming poll slot: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *DeviceFlowService) deleteDeviceCode(ctx context.Context, deviceCode string) error {
	_, err := s.Pool.Exec(ctx, `
		DELETE FROM public.oauth_device_codes
		WHERE device_code = $1`, deviceCode)
	return err
}
