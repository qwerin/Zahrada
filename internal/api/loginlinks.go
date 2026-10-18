package api

// One-time login links for contractors without e-mail: a manager creates the
// link (shown as a QR code or sent by SMS), the contractor opens it on their
// phone and is signed in (30-day sliding session). A new link signs the
// contractor out everywhere else.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/events"
	"github.com/qwerin/zahrada/internal/model"
)

// LoginLinkTTL is how long an unused login link stays valid.
const LoginLinkTTL = 7 * 24 * time.Hour

type LoginLinkOut struct {
	URL       string    `json:"url" doc:"Open on the contractor's phone (single use)"`
	ExpiresAt time.Time `json:"expires_at"`
}

type LoginLinkRequest struct {
	Token string `json:"token" minLength:"10" maxLength:"100"`
}

func (s *server) registerLoginLinks(public, account huma.API) {
	huma.Post(account, "/contractors/{id}/login-link", s.createLoginLink, status(http.StatusCreated), auth.ForManagers)
	huma.Post(public, "/api/auth/login-link", s.useLoginLink)
}

// placeholderEmail is the unique, never-deliverable address of a LinkOnly user.
func placeholderEmail() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "dodavatel-" + hex.EncodeToString(b) + "@bez-emailu.invalid"
}

func (s *server) createLoginLink(ctx context.Context, in *idPath) (*Out[LoginLinkOut], error) {
	acc, by := auth.AccountFrom(ctx), auth.UserFrom(ctx)
	plain, hash := auth.NewSecret()
	link := model.LoginLink{AccountID: acc.ID, TokenHash: hash, ExpiresAt: s.deps.Now().Add(LoginLinkTTL), CreatedBy: by.ID}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := s.loadContractor(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if c.UserID == nil {
			u := model.User{Email: placeholderEmail(), Name: c.Name, LinkOnly: true}
			if err := tx.Create(&u).Error; err != nil {
				return dbErr(err, "user")
			}
			if err := tx.Create(&model.Membership{UserID: u.ID, AccountID: acc.ID, Role: model.RoleContractor}).Error; err != nil {
				return dbErr(err, "membership")
			}
			if err := tx.Model(c).Update("user_id", u.ID).Error; err != nil {
				return dbErr(err, "contractor")
			}
			c.UserID = &u.ID
		} else {
			var u model.User
			if err := tx.First(&u, *c.UserID).Error; err != nil {
				return dbErr(err, "user")
			}
			if !u.LinkOnly {
				return conflict(CodeAlreadyLinked, "the contractor signs in with e-mail and password")
			}
			// a new link replaces the old ones and signs out other devices (lost phone)
			if err := tx.Where("user_id = ?", u.ID).Delete(&model.LoginLink{}).Error; err != nil {
				return dbErr(err, "login link")
			}
			if err := tx.Where("user_id = ?", u.ID).Delete(&model.Session{}).Error; err != nil {
				return dbErr(err, "session")
			}
		}
		// the membership may have been removed in the team settings meanwhile
		var n int64
		if err := tx.Model(&model.Membership{}).Where("user_id = ? AND account_id = ?", *c.UserID, acc.ID).Count(&n).Error; err != nil {
			return dbErr(err, "membership")
		}
		if n == 0 {
			if err := tx.Create(&model.Membership{UserID: *c.UserID, AccountID: acc.ID, Role: model.RoleContractor}).Error; err != nil {
				return dbErr(err, "membership")
			}
		}
		link.UserID = *c.UserID
		if err := tx.Create(&link).Error; err != nil {
			return dbErr(err, "login link")
		}
		return recordSimple(ctx, tx, events.ContractorInvited, events.SubjectContractor, c.ID,
			"Pro dodavatele "+c.Name+" vytvořen přihlašovací odkaz", map[string]any{"login_link": true})
	})
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(s.cfg.PublicURL, "/") + "/prihlaseni/" + plain
	return &Out[LoginLinkOut]{Body: LoginLinkOut{URL: url, ExpiresAt: link.ExpiresAt}}, nil
}

// useLoginLink signs the link's user in. The link works once.
func (s *server) useLoginLink(ctx context.Context, in *struct{ Body LoginLinkRequest }) (*meWithCookie, error) {
	key := "link:" + clientIP(ctx)
	if err := s.rateBlocked(s.limits.password, key); err != nil {
		return nil, err
	}
	invalidLink := func() error {
		s.rateFail(s.limits.password, key)
		return apiError(http.StatusBadRequest, CodeInvalidLink, "the login link is invalid, used or expired; ask your manager for a new one")
	}
	var user model.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var link model.LoginLink
		err := tx.Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", auth.HashSecret(in.Body.Token), s.deps.Now()).First(&link).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalidLink()
		}
		if err != nil {
			return dbErr(err, "login link")
		}
		now := s.deps.Now()
		res := tx.Model(&model.LoginLink{}).Where("id = ? AND used_at IS NULL", link.ID).Update("used_at", now)
		if res.Error != nil {
			return dbErr(res.Error, "login link")
		}
		if res.RowsAffected == 0 { // used concurrently
			return invalidLink()
		}
		if err := tx.First(&user, link.UserID).Error; err != nil {
			return dbErr(err, "user")
		}
		if !user.LinkOnly {
			return invalidLink()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, &user)
}
