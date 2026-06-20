package host

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/timothydodd/ntunl/internal/auth"
	"github.com/timothydodd/ntunl/internal/store"
)

// Default credentials used when bootstrapping the first admin and no override is
// provided. These are intentionally well-known — change the password immediately
// after first sign-in (the dashboard has a change-password form).
const (
	DefaultAdminUser     = "admin"
	DefaultAdminPassword = "admin"
)

// BootstrapAdmin creates an initial admin user when the database has no users.
// Credentials come from NTUNL_ADMIN_USER / NTUNL_ADMIN_PASSWORD, falling back to
// the default admin/admin login.
func BootstrapAdmin(log *slog.Logger, st *store.Store) error {
	n, err := st.CountUsers()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	username := os.Getenv("NTUNL_ADMIN_USER")
	if username == "" {
		username = DefaultAdminUser
	}
	password := os.Getenv("NTUNL_ADMIN_PASSWORD")
	usedDefault := false
	if password == "" {
		password = DefaultAdminPassword
		usedDefault = true
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := st.CreateUser(username, hash, store.RoleAdmin); err != nil {
		return err
	}

	if usedDefault {
		log.Warn("Created initial admin with the DEFAULT password",
			"username", username,
			"password", DefaultAdminPassword,
			"action", "sign in and change it now (Dashboard → Change password)")
	} else {
		log.Info("Created initial admin account", "username", username)
	}
	return nil
}

// CreateAdmin creates an admin with an explicit username/password (used by the
// `host create-admin` subcommand).
func CreateAdmin(st *store.Store, username, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := st.CreateUser(username, hash, store.RoleAdmin); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	return nil
}
