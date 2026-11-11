package rpc

// auth_bans.txt based request gate. A banned client cannot even request a
// verification code (auth.sendCode / resendCode / signIn / signUp), which
// blocks both login and registration. Lines in /root/gramsrv/auth_bans.txt:
//
//	ip <address>
//	phone <digits>
//	user <id>
//	authkey <raw auth key id hex>   (alias: hwid)
//
// Blank lines and # comments are ignored. The file is re-parsed at most every
// authBanReloadInterval, so edits apply without a daemon restart.

import (
	"bufio"
	"context"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	authBanFile          = "/root/gramsrv/auth_bans.txt"
	authBanReloadInterval = 15 * time.Second
)

type authBanSet struct {
	mu       sync.RWMutex
	loadedAt time.Time
	ips      map[string]struct{}
	phones   map[string]struct{}
	users    map[int64]struct{}
	keys     map[string]struct{}
}

var authBans = &authBanSet{}

func (s *authBanSet) emptyLocked() bool {
	return len(s.ips) == 0 && len(s.phones) == 0 && len(s.users) == 0 && len(s.keys) == 0
}

// reload parses the ban file and swaps the parsed sets atomically.
func (s *authBanSet) reload() {
	out := &authBanSet{
		loadedAt: time.Now(),
		ips:      map[string]struct{}{},
		phones:   map[string]struct{}{},
		users:    map[int64]struct{}{},
		keys:     map[string]struct{}{},
	}
	if f, err := os.Open(authBanFile); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 16*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			kind, rest := line, ""
			if idx := strings.IndexAny(line, " \t"); idx >= 0 {
				kind, rest = strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
			}
			switch strings.ToLower(kind) {
			case "ip":
				if rest != "" {
					out.ips[strings.TrimSpace(rest)] = struct{}{}
				}
			case "phone":
				if d := authBanDigits(rest); d != "" {
					out.phones[d] = struct{}{}
				}
			case "user":
				if id, err := strconv.ParseInt(rest, 10, 64); err == nil && id != 0 {
					out.users[id] = struct{}{}
				}
			case "authkey", "hwid":
				if rest != "" {
					out.keys[strings.ToLower(strings.TrimSpace(rest))] = struct{}{}
				}
			}
		}
		_ = f.Close()
	}
	// NOTE: never copy the whole struct (*s = *out): it would overwrite the
	// live RWMutex while other goroutines hold its read lock, causing
	// "sync: Unlock of unlocked RWMutex" fatals. Update fields under the lock.
	s.mu.Lock()
	s.loadedAt = out.loadedAt
	s.ips = out.ips
	s.phones = out.phones
	s.users = out.users
	s.keys = out.keys
	s.mu.Unlock()
}

func (s *authBanSet) current() *authBanSet {
	s.mu.RLock()
	fresh := time.Since(s.loadedAt) < authBanReloadInterval
	loaded := s.loadedAt
	s.mu.RUnlock()
	if !fresh {
		s.reload()
	}
	_ = loaded
	return s
}

func authBanDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// authBanReason returns the matching ban rule kind, or "" when unblocked.
func (r *Router) authBanReason(ctx context.Context, phone string) string {
	s := authBans.current()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.emptyLocked() {
		return ""
	}
	if ip, ok := ClientIPFrom(ctx); ok && ip != "" {
		if _, hit := s.ips[ip]; hit {
			return "ip:" + ip
		}
	}
	if id, ok := RawAuthKeyIDFrom(ctx); ok && id != ([8]byte{}) {
		if _, hit := s.keys[hex.EncodeToString(id[:])]; hit {
			return "hwid:" + hex.EncodeToString(id[:])
		}
	}
	if d := authBanDigits(phone); d != "" {
		if _, hit := s.phones[d]; hit {
			return "phone:" + d
		}
	}
	if userID, authorized, err := r.currentUserID(ctx); err == nil && authorized && userID != 0 {
		if _, hit := s.users[userID]; hit {
			return "user:" + strconv.FormatInt(userID, 10)
		}
	}
	return ""
}

// logAuthBan records a blocked auth attempt, including the device fingerprint
// (raw auth key id) and IP so an operator can extend the ban file.
func (r *Router) logAuthBan(ctx context.Context, phone, reason string) {
	fields := append(r.contextLogFields(ctx),
		zap.String("ban_reason", reason),
		zap.String("phone", phone),
	)
	r.log.Warn("auth request blocked by ban rule", fields...)
}

func (r *Router) authBanBlocked(ctx context.Context, phone string) bool {
	if reason := r.authBanReason(ctx, phone); reason != "" {
		r.logAuthBan(ctx, phone, reason)
		return true
	}
	return false
}
