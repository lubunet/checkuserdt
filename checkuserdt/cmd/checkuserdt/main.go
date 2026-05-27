package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPort       = 555
	defaultXrayConfig = "/usr/local/etc/xray/config.json"
	defaultDataDir    = "/root/checkuserdt/data"
	onlineTTL         = 120 * time.Second
)

type App struct {
	XrayConfig string
	DataDir    string
	Store      *OnlineStore
}

type XrayConfig struct {
	Inbounds []struct {
		Tag      string `json:"tag"`
		Protocol string `json:"protocol"`
		Settings struct {
			Clients []XrayClient `json:"clients"`
		} `json:"settings"`
	} `json:"inbounds"`
}

type XrayClient struct {
	Email string `json:"email"`
	ID    string `json:"id"`
	Level int    `json:"level"`
}

type UserInfo struct {
	Input            string       `json:"input"`
	ID               uint32       `json:"id"`
	Username         string       `json:"username"`
	UUID             string       `json:"uuid,omitempty"`
	XrayUser         string       `json:"xray_user,omitempty"`
	Mode             string       `json:"mode"`
	ExpirationDate   string       `json:"expiration_date"`
	ExpirationDays   int          `json:"expiration_days"`
	LimitConnections int          `json:"limit_connections"`
	CountConnections int          `json:"count_connections"`
	Status           string       `json:"status"`
	Traffic          *TrafficInfo `json:"traffic,omitempty"`
	Message          string       `json:"message,omitempty"`
}

type TrafficInfo struct {
	UplinkBytes   int64 `json:"uplink_bytes"`
	DownlinkBytes int64 `json:"downlink_bytes"`
	TotalBytes    int64 `json:"total_bytes"`
	Available     bool  `json:"available"`
}

type OnlineSession struct {
	Key           string `json:"key"`
	Username      string `json:"username"`
	UUID          string `json:"uuid,omitempty"`
	DeviceID      string `json:"device_id,omitempty"`
	DTunnelID     string `json:"dtunnel_id,omitempty"`
	VPNState      string `json:"vpn_state,omitempty"`
	ConfigID      string `json:"config_id,omitempty"`
	ConfigName    string `json:"config_name,omitempty"`
	LocalIP       string `json:"local_ip,omitempty"`
	PublicIP      string `json:"public_ip,omitempty"`
	NetworkName   string `json:"network_name,omitempty"`
	NetworkType   string `json:"network_type,omitempty"`
	NetworkExtra  string `json:"network_extra_info,omitempty"`
	Operator      string `json:"operator,omitempty"`
	Carrier       string `json:"carrier,omitempty"`
	AppVersion    string `json:"app_version,omitempty"`
	AndroidVer    string `json:"android_version,omitempty"`
	DownloadBytes int64  `json:"download_bytes,omitempty"`
	UploadBytes   int64  `json:"upload_bytes,omitempty"`
	ConnectedAt   int64  `json:"connected_at,omitempty"`
	FirstSeen     int64  `json:"first_seen"`
	LastSeen      int64  `json:"last_seen"`
	Category      string `json:"category"`
}

type OnlineStore struct {
	mu       sync.Mutex
	FilePath string
	Items    map[string]OnlineSession `json:"items"`
}

func main() {
	start := flag.Bool("start", false, "inicia servidor HTTP")
	listen := flag.String("listen", "0.0.0.0", "endereco de escuta")
	port := flag.Int("port", defaultPort, "porta HTTP")
	xrayConfig := flag.String("xray-config", defaultXrayConfig, "arquivo config.json do Xray")
	dataDir := flag.String("data-dir", defaultDataDir, "pasta de dados")
	flag.String("xray-api", "127.0.0.1:1085", "mantido por compatibilidade")
	flag.String("xray-bin", "/usr/local/bin/xray", "mantido por compatibilidade")
	flag.Parse()

	if !*start {
		fmt.Println("Uso: checkuserdt --start --listen 0.0.0.0 --port 555")
		return
	}

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("erro criando data-dir: %v", err)
	}

	store := NewOnlineStore(filepath.Join(*dataDir, "online.json"))
	if err := store.Load(); err != nil {
		log.Printf("aviso: nao foi possivel carregar online.json: %v", err)
	}

	app := &App{XrayConfig: *xrayConfig, DataDir: *dataDir, Store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.withCORS(app.handleRoot))
	mux.HandleFunc("/health", app.withCORS(app.handleHealth))
	mux.HandleFunc("/check", app.withCORS(app.handleCheckQuery))
	mux.HandleFunc("/check/", app.withCORS(app.handleCheckPath))
	mux.HandleFunc("/details/", app.withCORS(app.handleCheckPath))
	mux.HandleFunc("/stats/", app.withCORS(app.handleCheckPath))
	mux.HandleFunc("/users", app.withCORS(app.handleUsers))
	mux.HandleFunc("/count", app.withCORS(app.handleCount))
	mux.HandleFunc("/telemetry/heartbeat", app.withCORS(app.handleHeartbeat))
	mux.HandleFunc("/online/heartbeat", app.withCORS(app.handleHeartbeat))
	mux.HandleFunc("/online", app.withCORS(app.handleOnline))
	mux.HandleFunc("/online/categories", app.withCORS(app.handleOnlineCategories))
	mux.HandleFunc("/online/category", app.withCORS(app.handleOnlineCategory))

	addr := fmt.Sprintf("%s:%d", *listen, *port)
	log.Printf("checkuserdt ouvindo em http://%s", addr)
	log.Printf("config Xray: %s | dados: %s", *xrayConfig, *dataDir)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (a *App) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func (a *App) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":   "checkuserdt",
		"status": "online",
		"usage":  "configure este endereco base no app: http://IP_DA_VPS:555",
	})
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "checkuserdt", "port": 555})
}

func (a *App) handleCheckQuery(w http.ResponseWriter, r *http.Request) {
	input := strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("username"), r.URL.Query().Get("user"), r.URL.Query().Get("uuid"), r.URL.Query().Get("id")))
	if input == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "message": "informe username ou uuid"})
		return
	}
	a.writeCheck(w, input)
}

func (a *App) handleCheckPath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "message": "informe usuario ou uuid"})
		return
	}
	input := strings.TrimSpace(parts[1])
	a.writeCheck(w, input)
}

func (a *App) writeCheck(w http.ResponseWriter, input string) {
	info, err := a.ResolveUser(input)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"input": input, "status": "not_found", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *App) ResolveUser(input string) (*UserInfo, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("entrada vazia")
	}

	clients, _ := a.ListXrayClients()
	var matched *XrayClient
	mode := "linux_user"
	linuxUser := input
	uuid := ""

	for i := range clients {
		c := clients[i]
		if strings.EqualFold(c.ID, input) {
			matched = &c
			mode = "xray_uuid"
			linuxUser = c.Email
			uuid = c.ID
			break
		}
	}
	if matched == nil {
		for i := range clients {
			c := clients[i]
			if c.Email == input {
				matched = &c
				mode = "linux_user"
				linuxUser = c.Email
				uuid = c.ID
				break
			}
		}
	}
	if linuxUser == "" {
		return nil, fmt.Errorf("uuid encontrado no Xray, mas sem email/usuario vinculado")
	}

	expDate, days, status, err := getLinuxExpiration(linuxUser)
	if err != nil {
		return nil, err
	}

	countConn := countUserProcesses(linuxUser)
	info := &UserInfo{
		Input:            input,
		ID:               crc32.ChecksumIEEE([]byte(linuxUser)),
		Username:         linuxUser,
		UUID:             uuid,
		XrayUser:         linuxUser,
		Mode:             mode,
		ExpirationDate:   expDate,
		ExpirationDays:   days,
		LimitConnections: 0,
		CountConnections: countConn,
		Status:           status,
		Traffic:          &TrafficInfo{Available: false},
	}
	return info, nil
}

func (a *App) ListXrayClients() ([]XrayClient, error) {
	b, err := os.ReadFile(a.XrayConfig)
	if err != nil {
		return nil, err
	}
	var cfg XrayConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	var out []XrayClient
	for _, in := range cfg.Inbounds {
		for _, c := range in.Settings.Clients {
			if strings.TrimSpace(c.Email) == "" && strings.TrimSpace(c.ID) == "" {
				continue
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func (a *App) handleUsers(w http.ResponseWriter, r *http.Request) {
	clients, err := a.ListXrayClients()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "message": err.Error()})
		return
	}
	type row struct {
		Username       string `json:"username"`
		UUID           string `json:"uuid"`
		ExpirationDate string `json:"expiration_date"`
		ExpirationDays int    `json:"expiration_days"`
		Status         string `json:"status"`
	}
	var rows []row
	for _, c := range clients {
		if c.Email == "" {
			continue
		}
		exp, days, st, _ := getLinuxExpiration(c.Email)
		rows = append(rows, row{Username: c.Email, UUID: c.ID, ExpirationDate: exp, ExpirationDays: days, Status: st})
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(rows), "users": rows})
}

func (a *App) handleCount(w http.ResponseWriter, r *http.Request) {
	clients, _ := a.ListXrayClients()
	writeJSON(w, http.StatusOK, map[string]any{"xray_users": len(clients), "online": len(a.Store.List(true))})
}

func (a *App) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "message": "use POST"})
		return
	}
	var s OnlineSession
	if r.Method == http.MethodPost {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1024*128))
		_ = json.Unmarshal(body, &s)
	}
	q := r.URL.Query()
	fillIfEmpty(&s.Username, firstNonEmpty(q.Get("username"), q.Get("user")))
	fillIfEmpty(&s.UUID, firstNonEmpty(q.Get("uuid"), q.Get("id")))
	fillIfEmpty(&s.DeviceID, q.Get("device_id"))
	fillIfEmpty(&s.VPNState, q.Get("vpn_state"))
	fillIfEmpty(&s.ConfigID, q.Get("config_id"))
	fillIfEmpty(&s.ConfigName, q.Get("config_name"))
	fillIfEmpty(&s.LocalIP, q.Get("local_ip"))
	fillIfEmpty(&s.NetworkName, q.Get("network_name"))
	fillIfEmpty(&s.NetworkType, q.Get("network_type"))
	fillIfEmpty(&s.Operator, q.Get("operator"))
	fillIfEmpty(&s.Carrier, q.Get("carrier"))
	fillIfEmpty(&s.DTunnelID, q.Get("dtunnel_id"))

	if s.UUID != "" && s.Username == "" {
		if info, err := a.ResolveUser(s.UUID); err == nil {
			s.Username = info.Username
		}
	}
	if s.Username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "message": "informe username ou uuid"})
		return
	}
	if s.PublicIP == "" {
		s.PublicIP = clientIP(r)
	}
	now := time.Now().Unix()
	if s.FirstSeen == 0 {
		s.FirstSeen = now
	}
	s.LastSeen = now
	s.Category = detectCategory(s)
	s.Key = makeSessionKey(s)
	a.Store.Upsert(s)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "session": s})
}

func (a *App) handleOnline(w http.ResponseWriter, r *http.Request) {
	items := a.Store.List(r.URL.Query().Get("all") != "1")
	writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "ttl_seconds": int(onlineTTL.Seconds()), "users": items})
}

func (a *App) handleOnlineCategories(w http.ResponseWriter, r *http.Request) {
	counts := map[string]int{}
	for _, s := range a.Store.List(true) {
		counts[s.Category]++
	}
	order := []string{"Vivo", "Tim", "Claro", "Oi", "Wi-Fi", "Dados moveis", "Outros"}
	if r.URL.Query().Get("format") == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, name := range order {
			if counts[name] > 0 {
				fmt.Fprintf(w, "%s|%d\n", name, counts[name])
			}
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": counts, "order": order})
}

func (a *App) handleOnlineCategory(w http.ResponseWriter, r *http.Request) {
	cat := r.URL.Query().Get("category")
	format := r.URL.Query().Get("format")
	var rows []OnlineSession
	for _, s := range a.Store.List(true) {
		if cat == "" || strings.EqualFold(s.Category, cat) {
			rows = append(rows, s)
		}
	}
	if format == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if len(rows) == 0 {
			fmt.Fprintln(w, "Nenhum usuario online nesta categoria.")
			return
		}
		for i, s := range rows {
			fmt.Fprintf(w, "[%d] Usuario: %s\n", i+1, s.Username)
			if s.UUID != "" {
				fmt.Fprintf(w, "    UUID: %s\n", s.UUID)
			}
			if s.ConfigName != "" || s.ConfigID != "" {
				fmt.Fprintf(w, "    Config: %s %s\n", s.ConfigName, s.ConfigID)
			}
			if s.LocalIP != "" {
				fmt.Fprintf(w, "    IP local: %s\n", s.LocalIP)
			}
			if s.PublicIP != "" {
				fmt.Fprintf(w, "    IP publico: %s\n", s.PublicIP)
			}
			if s.NetworkName != "" || s.NetworkType != "" {
				fmt.Fprintf(w, "    Rede: %s %s\n", s.NetworkName, s.NetworkType)
			}
			if s.Operator != "" || s.Carrier != "" {
				fmt.Fprintf(w, "    Operadora: %s %s\n", s.Operator, s.Carrier)
			}
			if s.VPNState != "" {
				fmt.Fprintf(w, "    VPN: %s\n", s.VPNState)
			}
			fmt.Fprintf(w, "    Ultimo sinal: %s\n\n", time.Unix(s.LastSeen, 0).Format("02/01/2006 15:04:05"))
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"category": cat, "count": len(rows), "users": rows})
}

func NewOnlineStore(path string) *OnlineStore {
	return &OnlineStore{FilePath: path, Items: map[string]OnlineSession{}}
}

func (s *OnlineStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.FilePath)
	if err != nil {
		return err
	}
	var tmp OnlineStore
	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}
	if tmp.Items != nil {
		s.Items = tmp.Items
	}
	return nil
}

func (s *OnlineStore) Save() error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.FilePath, b, 0644)
}

func (s *OnlineStore) Upsert(item OnlineSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.Items[item.Key]; ok && item.FirstSeen == 0 {
		item.FirstSeen = old.FirstSeen
	}
	if old, ok := s.Items[item.Key]; ok && item.FirstSeen == item.LastSeen {
		item.FirstSeen = old.FirstSeen
	}
	s.Items[item.Key] = item
	_ = s.Save()
}

func (s *OnlineStore) List(onlyOnline bool) []OnlineSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	var out []OnlineSession
	for _, item := range s.Items {
		if onlyOnline && now-item.LastSeen > int64(onlineTTL.Seconds()) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func getLinuxExpiration(username string) (string, int, string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", 0, "not_found", fmt.Errorf("usuario vazio")
	}
	if !linuxUserExists(username) {
		return "", 0, "not_found", fmt.Errorf("usuario %s nao encontrado no Linux", username)
	}

	file, err := os.Open("/etc/shadow")
	if err != nil {
		return "", 0, "error", fmt.Errorf("nao foi possivel ler /etc/shadow: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) < 8 || parts[0] != username {
			continue
		}
		expireField := strings.TrimSpace(parts[7])
		if expireField == "" || expireField == "-1" {
			return "Nunca", 9999, "active", nil
		}
		expireDays, err := strconv.ParseInt(expireField, 10, 64)
		if err != nil || expireDays <= 0 {
			return "Nunca", 9999, "active", nil
		}
		expireTime := time.Unix(expireDays*86400, 0).Local()
		now := time.Now()
		daysLeft := int(expireTime.Sub(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())).Hours() / 24)
		status := "active"
		if daysLeft < 0 {
			status = "expired"
		}
		return expireTime.Format("02/01/2006"), daysLeft, status, nil
	}
	return "", 0, "not_found", fmt.Errorf("usuario %s nao encontrado em /etc/shadow", username)
}

func linuxUserExists(username string) bool {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	prefix := username + ":"
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), prefix) {
			return true
		}
	}
	return false
}

func countUserProcesses(username string) int {
	cmd := exec.Command("pgrep", "-u", username)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	lines := strings.Fields(string(out))
	return len(lines)
}

func detectCategory(s OnlineSession) string {
	v := strings.ToLower(strings.Join([]string{s.Operator, s.Carrier, s.NetworkName, s.NetworkType, s.NetworkExtra}, " "))
	replacer := strings.NewReplacer("á", "a", "à", "a", "ã", "a", "â", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c")
	v = replacer.Replace(v)
	switch {
	case strings.Contains(v, "vivo"):
		return "Vivo"
	case strings.Contains(v, "tim"):
		return "Tim"
	case strings.Contains(v, "claro"):
		return "Claro"
	case strings.Contains(v, "oi"):
		return "Oi"
	case strings.Contains(v, "wifi") || strings.Contains(v, "wi-fi") || strings.Contains(v, "wlan"):
		return "Wi-Fi"
	case strings.Contains(v, "mobile") || strings.Contains(v, "cell") || strings.Contains(v, "dados"):
		return "Dados moveis"
	default:
		return "Outros"
	}
}

func makeSessionKey(s OnlineSession) string {
	base := firstNonEmpty(s.DeviceID, s.UUID, s.Username, s.PublicIP)
	h := sha1.Sum([]byte(base))
	return hex.EncodeToString(h[:])[:16]
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func fillIfEmpty(target *string, value string) {
	if *target == "" && strings.TrimSpace(value) != "" {
		*target = strings.TrimSpace(value)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
