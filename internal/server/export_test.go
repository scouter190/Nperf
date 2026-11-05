package server

// SessionDataConns devuelve cuántas conexiones de datos ha contado la sesión (para tests).
func (s *Server) SessionDataConns(id string) int64 {
	s.mu.Lock()
	ss := s.sessions[id]
	s.mu.Unlock()
	if ss == nil {
		return 0
	}
	return ss.n.Load()
}
