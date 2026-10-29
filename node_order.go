package main

import "net/http"

// reorderNodes changes only configuration order; connections and log state stay live.
func (s *server) reorderNodes(w http.ResponseWriter, r *http.Request) {
	if !requireFileBackend(w, r) {
		return
	}
	var request struct {
		Order []string `json:"order"`
	}
	if err := decodeDatabaseRequest(w, r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid node order"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := make(map[string]Node, len(s.nodes))
	for _, node := range s.nodes {
		byID[node.ID] = node
	}
	ordered := make([]Node, 0, len(s.nodes))
	for _, id := range request.Order {
		if node, ok := byID[id]; ok {
			ordered = append(ordered, node)
			delete(byID, id)
		}
	}
	// Stale clients cannot remove nodes added since their last refresh.
	for _, node := range s.nodes {
		if _, ok := byID[node.ID]; ok {
			ordered = append(ordered, node)
		}
	}
	ids := make([]string, 0, len(ordered))
	storedIDs := make([]int64, 0, len(ordered))
	for _, node := range ordered {
		ids = append(ids, node.ID)
		if node.dbID > 0 {
			storedIDs = append(storedIDs, node.dbID)
		}
	}
	if s.store != nil {
		if err := s.store.reorderNodes(storedIDs); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save node order failed", "detail": err.Error()})
			return
		}
	}
	s.nodes = ordered
	writeJSON(w, http.StatusOK, map[string]any{"order": ids})
}

func (c *configStore) reorderNodes(ids []int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLockedIfNeeded(); err != nil {
		return err
	}
	byID := make(map[int64]storedNode, len(c.nodes))
	for _, node := range c.nodes {
		byID[node.ID] = node
	}
	ordered := make([]storedNode, 0, len(c.nodes))
	for _, id := range ids {
		if node, ok := byID[id]; ok {
			ordered = append(ordered, node)
			delete(byID, id)
		}
	}
	for _, node := range c.nodes {
		if _, ok := byID[node.ID]; ok {
			ordered = append(ordered, node)
		}
	}
	previous := c.nodes
	c.nodes = ordered
	if err := c.saveNodesLocked(); err != nil {
		c.nodes = previous
		return err
	}
	return nil
}
