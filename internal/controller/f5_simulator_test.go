/*
MIT License

Copyright (c) 2022 Carlos Eduardo de Paula

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package controllers

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	simPartitionPath = "/Common/"
	simPartitionURL  = "~Common~"

	// Pool member sessions
	simSessionMonitorEnabled = "monitor-enabled"
	simSessionUserEnabled    = "user-enabled"
	simSessionUserDisabled   = "user-disabled"

	// Pool member changes recorded by the simulator
	simCreated  = "created"
	simEnabled  = "enabled"
	simDisabled = "disabled"
	simDeleted  = "deleted"
)

// f5Simulator is a minimal in-memory F5 BIG-IP iControl REST API serving the calls done by the F5 provider.
// Like a real BIG-IP, a pool member is only found through a partition-qualified pool and member name
// (eg. /mgmt/tm/ltm/pool/~Common~pool/members/~Common~10.0.0.1:80), otherwise it answers "Object not found".
// A live BIG-IP still applied a status change sent to the bare member name while answering "Object not found";
// the simulator rejects the request outright.
type f5Simulator struct {
	server   *httptest.Server
	mu       sync.Mutex
	monitors map[string][]byte
	pools    map[string]*simPool
	nodes    map[string]bool
	virtuals map[string]*simVirtual
	events   []simEvent
}

type simPool struct {
	monitor string
	members map[string]*simMember
}

type simMember struct {
	address string
	session string
	state   string
}

// simEvent is a pool member change applied by the simulator
type simEvent struct {
	action string
	member string
	at     time.Time
}

// simRequest holds the request body fields used by the simulator
type simRequest struct {
	Name        string `json:"name"`
	Monitor     string `json:"monitor"`
	Session     string `json:"session"`
	State       string `json:"state"`
	Destination string `json:"destination"`
	Pool        string `json:"pool"`
}

type simPoolResponse struct {
	Name     string `json:"name"`
	FullPath string `json:"fullPath"`
	Monitor  string `json:"monitor,omitempty"`
}

type simMemberResponse struct {
	Name     string `json:"name"`
	FullPath string `json:"fullPath"`
	Address  string `json:"address"`
	Session  string `json:"session"`
	State    string `json:"state"`
}

type simNode struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type simVirtual struct {
	Name        string `json:"name"`
	Destination string `json:"destination"`
	Pool        string `json:"pool"`
}

type simCollection struct {
	Items any `json:"items"`
}

type simError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newF5Simulator() *f5Simulator {
	s := &f5Simulator{
		monitors: map[string][]byte{},
		pools:    map[string]*simPool{},
		nodes:    map[string]bool{},
		virtuals: map[string]*simVirtual{},
	}
	s.server = httptest.NewTLSServer(s)
	return s
}

func (s *f5Simulator) close() {
	s.server.Close()
}

// hostPort returns the simulator API address in the ExternalLoadBalancer provider format
func (s *f5Simulator) hostPort() (string, int, error) {
	u, err := url.Parse(s.server.URL)
	if err != nil {
		return "", 0, err
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return "", 0, err
	}
	p, err := strconv.Atoi(port)
	return u.Scheme + "://" + host, p, err
}

// member returns a copy of the pool member and whether it exists
func (s *f5Simulator) member(pool, name string) (simMember, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.pools[pool]; ok {
		if m, ok := p.members[name]; ok {
			return *m, true
		}
	}
	return simMember{}, false
}

// memberSession returns the pool member session, empty when the member does not exist
func (s *f5Simulator) memberSession(pool, name string) string {
	m, _ := s.member(pool, name)
	return m.session
}

func (s *f5Simulator) poolExists(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.pools[name]
	return ok
}

// eventTime returns when the action was first applied to the member
func (s *f5Simulator) eventTime(action, member string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.events {
		if e.action == action && e.member == member {
			return e.at, true
		}
	}
	return time.Time{}, false
}

// count returns how many times the action was applied to the member
func (s *f5Simulator) count(action, member string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.events {
		if e.action == action && e.member == member {
			n++
		}
	}
	return n
}

func (s *f5Simulator) record(action, member string) {
	s.events = append(s.events, simEvent{action: action, member: member, at: time.Now()})
}

func simReply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func simNotFound(w http.ResponseWriter, name string) {
	simReply(w, http.StatusNotFound, simError{Code: http.StatusNotFound, Message: "Object not found - " + name})
}

// simName strips the partition from a name in the URL and reports whether it was partition-qualified
func simName(segment string) (string, bool) {
	name := strings.TrimPrefix(segment, simPartitionURL)
	return name, name != segment
}

func (s *f5Simulator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, _ := io.ReadAll(r.Body)
	var req simRequest
	if len(data) > 0 {
		_ = json.Unmarshal(data, &req)
	}

	path := strings.Split(strings.TrimPrefix(r.URL.Path, "/mgmt/tm/ltm/"), "/")
	switch path[0] {
	case "monitor":
		s.serveMonitor(w, r.Method, path[1:], req, data)
	case "pool":
		s.servePool(w, r.Method, path[1:], req)
	case "node":
		s.serveNode(w, r.Method, path[1:], req)
	case "virtual":
		s.serveVirtual(w, r.Method, path[1:], req)
	default:
		simNotFound(w, r.URL.Path)
	}
}

// serveMonitor serves /monitor/<type> and /monitor/<type>/<name>
func (s *f5Simulator) serveMonitor(w http.ResponseWriter, method string, path []string, req simRequest, data []byte) {
	if len(path) == 1 && method == http.MethodPost {
		s.monitors[req.Name] = data
		simReply(w, http.StatusOK, json.RawMessage(data))
		return
	}
	if len(path) != 2 {
		simNotFound(w, strings.Join(path, "/"))
		return
	}
	name, _ := simName(path[1])
	monitor, ok := s.monitors[name]
	if !ok {
		simNotFound(w, name)
		return
	}
	switch method {
	case http.MethodDelete:
		delete(s.monitors, name)
	case http.MethodPut, http.MethodPatch:
		s.monitors[name] = data
		monitor = data
	}
	simReply(w, http.StatusOK, json.RawMessage(monitor))
}

// servePool serves /pool, /pool/<name>, /pool/<name>/members and /pool/<name>/members/<member>
func (s *f5Simulator) servePool(w http.ResponseWriter, method string, path []string, req simRequest) {
	if len(path) == 0 {
		if method != http.MethodPost {
			simNotFound(w, "pool")
			return
		}
		s.pools[req.Name] = &simPool{members: map[string]*simMember{}}
		simReply(w, http.StatusOK, simPoolResponse{Name: req.Name, FullPath: simPartitionPath + req.Name})
		return
	}

	name, qualified := simName(path[0])
	pool, ok := s.pools[name]
	if !ok {
		simNotFound(w, name)
		return
	}
	switch {
	case len(path) == 1:
		switch method {
		case http.MethodDelete:
			delete(s.pools, name)
		case http.MethodPut, http.MethodPatch:
			if req.Monitor != "" {
				pool.monitor = simPartitionPath + strings.TrimPrefix(req.Monitor, simPartitionPath)
			}
		}
		simReply(w, http.StatusOK, simPoolResponse{Name: name, FullPath: simPartitionPath + name, Monitor: pool.monitor})
	case len(path) == 2 && path[1] == "members":
		s.servePoolMembers(w, method, pool, req)
	case len(path) == 3 && path[1] == "members" && qualified:
		s.servePoolMember(w, method, pool, path[2], req)
	default:
		simNotFound(w, path[len(path)-1])
	}
}

func (s *f5Simulator) servePoolMembers(w http.ResponseWriter, method string, pool *simPool, req simRequest) {
	if method == http.MethodPost {
		pool.members[req.Name] = &simMember{address: strings.Split(req.Name, ":")[0], session: simSessionMonitorEnabled, state: "up"}
		s.record(simCreated, req.Name)
		simReply(w, http.StatusOK, req)
		return
	}
	names := make([]string, 0, len(pool.members))
	for n := range pool.members {
		names = append(names, n)
	}
	sort.Strings(names)
	items := make([]simMemberResponse, 0, len(names))
	for _, n := range names {
		items = append(items, simMemberJSON(n, pool.members[n]))
	}
	simReply(w, http.StatusOK, simCollection{Items: items})
}

func (s *f5Simulator) servePoolMember(w http.ResponseWriter, method string, pool *simPool, segment string, req simRequest) {
	name, qualified := simName(segment)
	member, ok := pool.members[name]
	if !ok || !qualified {
		simNotFound(w, name)
		return
	}
	switch method {
	case http.MethodDelete:
		delete(pool.members, name)
		s.record(simDeleted, name)
	case http.MethodPut, http.MethodPatch:
		if req.Session != "" {
			member.session = req.Session
			if req.Session == simSessionUserDisabled {
				s.record(simDisabled, name)
			} else {
				s.record(simEnabled, name)
			}
		}
		if req.State != "" {
			member.state = req.State
		}
	}
	simReply(w, http.StatusOK, simMemberJSON(name, member))
}

func simMemberJSON(name string, m *simMember) simMemberResponse {
	return simMemberResponse{Name: name, FullPath: simPartitionPath + name, Address: m.address, Session: m.session, State: m.state}
}

// serveNode serves /node and /node/<name>
func (s *f5Simulator) serveNode(w http.ResponseWriter, method string, path []string, req simRequest) {
	if len(path) == 0 && method == http.MethodPost {
		s.nodes[req.Name] = true
		simReply(w, http.StatusOK, simNode{Name: req.Name, Address: req.Name})
		return
	}
	if len(path) != 1 {
		simNotFound(w, "node")
		return
	}
	name, _ := simName(path[0])
	if !s.nodes[name] {
		simNotFound(w, name)
		return
	}
	if method == http.MethodDelete {
		delete(s.nodes, name)
	}
	simReply(w, http.StatusOK, simNode{Name: name, Address: name})
}

// serveVirtual serves /virtual, /virtual/<name> and its profiles and policies sub-collections
func (s *f5Simulator) serveVirtual(w http.ResponseWriter, method string, path []string, req simRequest) {
	if len(path) == 0 {
		if method != http.MethodPost {
			simNotFound(w, "virtual")
			return
		}
		s.virtuals[req.Name] = &simVirtual{Name: req.Name, Destination: simPartitionPath + req.Destination, Pool: req.Pool}
		simReply(w, http.StatusOK, s.virtuals[req.Name])
		return
	}

	name, _ := simName(path[0])
	vs, ok := s.virtuals[name]
	if !ok {
		simNotFound(w, name)
		return
	}
	if len(path) == 2 {
		simReply(w, http.StatusOK, simCollection{Items: []any{}})
		return
	}
	switch method {
	case http.MethodDelete:
		delete(s.virtuals, name)
	case http.MethodPut, http.MethodPatch:
		if req.Destination != "" {
			vs.Destination = simPartitionPath + strings.TrimPrefix(req.Destination, simPartitionPath)
		}
		if req.Pool != "" {
			vs.Pool = req.Pool
		}
	}
	simReply(w, http.StatusOK, vs)
}
