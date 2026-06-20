package store

import "database/sql"

// OpenTunnelEvent records a new connection and returns the event id.
func (s *Store) OpenTunnelEvent(userID int64, subdomain, remoteAddr string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO tunnel_events (user_id, subdomain, remote_addr, connected_at) VALUES (?, ?, ?, ?)`,
		userID, subdomain, remoteAddr, now(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CloseTunnelEvent stamps the disconnect time on an event.
func (s *Store) CloseTunnelEvent(id int64) error {
	_, err := s.db.Exec(`UPDATE tunnel_events SET disconnected_at = ? WHERE id = ?`, now(), id)
	return err
}

// RecentEventsByUser returns a user's most recent connection events.
func (s *Store) RecentEventsByUser(userID int64, limit int) ([]TunnelEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, subdomain, remote_addr, connected_at, disconnected_at
		   FROM tunnel_events WHERE user_id = ? ORDER BY connected_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TunnelEvent
	for rows.Next() {
		var e TunnelEvent
		var disc sql.NullString
		if err := rows.Scan(&e.ID, &e.UserID, &e.Subdomain, &e.RemoteAddr, &e.ConnectedAt, &disc); err != nil {
			return nil, err
		}
		e.DisconnectedAt = disc.String
		out = append(out, e)
	}
	return out, rows.Err()
}
