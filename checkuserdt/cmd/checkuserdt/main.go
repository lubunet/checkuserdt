package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	appName        = "checkuserdt"
	defaultPort    = 555
	defaultDataDir = "/root/checkuserdt/data"
)

type App struct {
	Listen     string
	Port       int
	XrayConfig string
	XrayAPI    string
	XrayBin    string
	DataDir    string
	Devices    *DeviceStore
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

type AccountInfo struct {
	Input          string       `json:"input"`
	ID             int          `json:"id"`
	Username       string       `json:"username"`
	UUID           string       `json:"uuid,omitempty"`
	XrayUser       string       `json:"xray_user,omitempty"`
	Mode           string       `json:"mode"`
	ExpirationDate string       `json:"expiration_date"`
	ExpirationDays int          `json:"expiration_days"`
	Limit          int          `json:"limit_connections"`
	Connections    int          `json:"count_connections"`
	Status         string       `json:"status"`
	DeviceID       string       `json:"device_id,omitempty"`
	Traffic        *TrafficInfo `json:"traffic,omitempty"`
	Message        string       `json:"message,omitempty"`
	CheckedAt      string       `json:"checked_at"`
}

type TrafficInfo struct {
	UplinkBytes   int64 `json:"uplink_bytes"`
	DownlinkBytes int64 `json:"downlink_bytes"`
	TotalBytes    int64 `json:"total_bytes"`
	Available     bool  `json:"available"`
}

type DeviceStore struct {
	Path string
	Mu   sync.Mutex
}

type DevicesDB struct {
	Users map[string]map[string]int64 `json:"users"`
}

func main() {
	start := flag.Bool("start", false, "inicia o servidor HTTP")
	listen := flag.String("listen", "0.0.0.0", "endereço de escuta")
	port := flag.Int("port", defaultPort, "porta HTTP")
	xrayConfig := flag.String("xray-config", "/usr/local/etc/xray/config.json", "caminho do config.json do Xray")
	xrayAPI := flag.String("xray-api", "127.0.0.1:1085", "endereço da API gRPC do Xray")
	xrayBin := flag.String("xray-bin", "/usr/local/bin/xray", "binário do Xray")
	dataDir := flag.String("data-dir", defaultDataDir, "diretório de dados do checkuser")
	flag.Parse()

	if !*start {
		fmt.Printf("%s instalado. Use: %s --start --port 555\n", appName, os.Args[0])
		return
	}

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("erro criando data-dir %s: %v", *dataDir, err)
	}

	app := &App{
		Listen:     *listen,
		Port:       *port,
		XrayConfig: *xrayConfig,
		XrayAPI:    *xrayAPI,
		XrayBin:    *xrayBin,
		DataDir:    *dataDir,
		Devices:    &DeviceStore{Path: filepath.Join(*dataDir, "devices.json")},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.route)

	addr := fmt.Sprintf("%s:%d", app.Listen, app.Port)
	log.Printf("%s ouvindo em http://%s", appName, addr)
	log.Printf("config Xray: %s | API Xray: %s", app.XrayConfig, app.XrayAPI)
	log.Fatal(http.ListenAndServe(addr, withCORS(mux)))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) route(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}

	if len(parts) == 0 {
		a.handleIndex(w, r)
		return
	}

	switch parts[0] {
	case "health":
		a.handleHealth(w, r)
	case "check", "details":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "informe usuario ou uuid. Ex: /check/usuario ou /check/uuid"})
			return
		}
		a.handleCheck(w, r, parts[1])
	case "stats":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "informe usuario ou uuid. Ex: /stats/usuario"})
			return
		}
		a.handleStats(w, r, parts[1])
	case "users":
		a.handleUsers(w, r)
	case "count":
		a.handleCount(w, r)
	case "devices":
		a.handleDevices(w, r, parts)
	case "panel":
		a.handlePanel(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "rota não encontrada"})
	}
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	base := baseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"service": appName,
		"status":  "online",
		"routes": []string{
			base + "/health",
			base + "/check/USUARIO_OU_UUID",
			base + "/details/USUARIO_OU_UUID",
			base + "/stats/USUARIO_OU_UUID",
			base + "/users",
			base + "/count",
			base + "/panel",
		},
	})
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"service":     appName,
		"port":        a.Port,
		"xray_config": a.XrayConfig,
		"checked_at":  time.Now().Format(time.RFC3339),
	})
}

func (a *App) handleCheck(w http.ResponseWriter, r *http.Request, input string) {
	deviceID := strings.TrimSpace(r.URL.Query().Get("deviceId"))
	info, err := a.BuildAccountInfo(input, deviceID, true)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error(), "input": input})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request, input string) {
	client, linuxUser, mode, err := a.ResolveInput(input)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error(), "input": input})
		return
	}

	traffic, _ := a.XrayStats(linuxUser)
	writeJSON(w, http.StatusOK, map[string]any{
		"input":      input,
		"mode":       mode,
		"username":   chooseUsername(input, client),
		"uuid":       client.ID,
		"xray_user":  linuxUser,
		"traffic":    traffic,
		"checked_at": time.Now().Format(time.RFC3339),
	})
}

func (a *App) handleUsers(w http.ResponseWriter, r *http.Request) {
	clients, err := a.ReadXrayClients()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	items := []AccountInfo{}
	for _, c := range clients {
		user := strings.TrimSpace(c.Email)
		if user == "" {
			continue
		}
		info, err := a.BuildAccountInfo(c.ID, "", false)
		if err == nil {
			items = append(items, *info)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "users": items})
}

func (a *App) handleCount(w http.ResponseWriter, r *http.Request) {
	count := countAllSSHConnections()
	writeJSON(w, http.StatusOK, map[string]any{
		"count_connections": count,
		"checked_at":        time.Now().Format(time.RFC3339),
	})
}

func (a *App) handleDevices(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 || parts[1] == "list" {
		writeJSON(w, http.StatusOK, a.Devices.Load())
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "rota não encontrada"})
}

func (a *App) handlePanel(w http.ResponseWriter, r *http.Request) {
	base := baseURL(r)
	html := `<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>CheckUserDT</title><style>
body{font-family:Arial,sans-serif;background:#0f172a;color:#e5e7eb;margin:0;padding:24px}.card{max-width:980px;margin:auto;background:#111827;border:1px solid #334155;border-radius:18px;padding:22px;box-shadow:0 10px 30px #0006}h1{margin:0 0 8px}code,input{background:#020617;color:#a7f3d0;border:1px solid #334155;border-radius:8px;padding:8px}input{width:75%;color:white}.btn{padding:9px 12px;border:0;border-radius:8px;background:#22c55e;color:#052e16;font-weight:700;cursor:pointer}pre{white-space:pre-wrap;background:#020617;border-radius:12px;padding:16px;overflow:auto}.muted{color:#94a3b8}.grid{display:grid;gap:12px;grid-template-columns:repeat(auto-fit,minmax(220px,1fr))}.box{background:#020617;border:1px solid #1e293b;border-radius:12px;padding:14px}</style></head>
<body><div class="card"><h1>CheckUserDT</h1><p class="muted">Consulta usuário normal e UUID do Xray/V2ray.</p>
<div class="grid"><div class="box"><b>Health</b><br><code>` + base + `/health</code></div><div class="box"><b>App</b><br><code>` + base + `/check/USUARIO_OU_UUID</code></div></div>
<p><input id="q" placeholder="Digite usuário ou UUID"><button class="btn" onclick="check()">Consultar</button></p><pre id="out">Resultado aparece aqui...</pre>
<script>
async function check(){let q=document.getElementById('q').value.trim(); if(!q) return; let r=await fetch('/check/'+encodeURIComponent(q)); document.getElementById('out').textContent=JSON.stringify(await r.json(),null,2)}
</script></div></body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (a *App) BuildAccountInfo(input, deviceID string, includeTraffic bool) (*AccountInfo, error) {
	client, linuxUser, mode, err := a.ResolveInput(input)
	if err != nil {
		return nil, err
	}

	expDate, days, status, err := linuxExpiration(linuxUser)
	if err != nil {
		return nil, err
	}

	limit := detectLimit(linuxUser, a.DataDir)
	connections := 0
	if deviceID != "" {
		_ = a.Devices.Register(linuxUser, deviceID)
		connections = a.Devices.Count(linuxUser)
	} else {
		connections = countSSHConnections(linuxUser)
	}

	var traffic *TrafficInfo
	if includeTraffic {
		traffic, _ = a.XrayStats(linuxUser)
	}

	usernameField := input
	if mode == "linux_user" {
		usernameField = linuxUser
	}

	return &AccountInfo{
		Input:          input,
		ID:             stableID(linuxUser),
		Username:       usernameField,
		UUID:           client.ID,
		XrayUser:       client.Email,
		Mode:           mode,
		ExpirationDate: expDate,
		ExpirationDays: days,
		Limit:          limit,
		Connections:    connections,
		Status:         status,
		DeviceID:       deviceID,
		Traffic:        traffic,
		CheckedAt:      time.Now().Format(time.RFC3339),
	}, nil
}

func (a *App) ResolveInput(input string) (XrayClient, string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return XrayClient{}, "", "", errors.New("entrada vazia")
	}

	clients, _ := a.ReadXrayClients()
	for _, c := range clients {
		if strings.EqualFold(c.ID, input) {
			if c.Email == "" {
				return c, "", "", errors.New("uuid encontrado, mas sem email/usuario no Xray")
			}
			return c, c.Email, "xray_uuid", nil
		}
	}

	// Se vier o usuário normal, tenta encontrar o UUID correspondente para enriquecer a resposta.
	for _, c := range clients {
		if c.Email == input {
			return c, input, "linux_user", nil
		}
	}

	if linuxUserExists(input) {
		return XrayClient{Email: input}, input, "linux_user", nil
	}

	return XrayClient{}, "", "", fmt.Errorf("usuario ou uuid não encontrado: %s", input)
}

func (a *App) ReadXrayClients() ([]XrayClient, error) {
	b, err := os.ReadFile(a.XrayConfig)
	if err != nil {
		return nil, fmt.Errorf("não foi possível ler %s: %w", a.XrayConfig, err)
	}
	var cfg XrayConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("config Xray inválido: %w", err)
	}
	out := []XrayClient{}
	seen := map[string]bool{}
	for _, inbound := range cfg.Inbounds {
		for _, c := range inbound.Settings.Clients {
			c.Email = strings.TrimSpace(c.Email)
			c.ID = strings.TrimSpace(c.ID)
			key := c.Email + "|" + c.ID
			if c.Email == "" && c.ID == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, c)
		}
	}
	return out, nil
}

func (a *App) XrayStats(email string) (*TrafficInfo, error) {
	info := &TrafficInfo{Available: false}
	if email == "" || a.XrayBin == "" || a.XrayAPI == "" {
		return info, errors.New("email/xray-api vazio")
	}
	if _, err := os.Stat(a.XrayBin); err != nil {
		return info, err
	}

	patterns := [][]string{
		{"api", "statsquery", "--server=" + a.XrayAPI, "-pattern", "user>>>" + email + ">>>traffic>>>"},
		{"api", "statsquery", "--server", a.XrayAPI, "-pattern", "user>>>" + email + ">>>traffic>>>"},
		{"api", "statsquery", "--server=" + a.XrayAPI, "--pattern", "user>>>" + email + ">>>traffic>>>"},
	}

	var output []byte
	var lastErr error
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	for _, args := range patterns {
		cmd := exec.CommandContext(ctx, a.XrayBin, args...)
		output, lastErr = cmd.CombinedOutput()
		if lastErr == nil && len(output) > 0 {
			break
		}
	}
	if lastErr != nil && len(output) == 0 {
		return info, lastErr
	}

	var parsed struct {
		Stat []struct {
			Name  string `json:"name"`
			Value int64  `json:"value"`
		} `json:"stat"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return info, err
	}
	for _, s := range parsed.Stat {
		if strings.Contains(s.Name, ">>>uplink") {
			info.UplinkBytes = s.Value
		}
		if strings.Contains(s.Name, ">>>downlink") {
			info.DownlinkBytes = s.Value
		}
	}
	info.TotalBytes = info.UplinkBytes + info.DownlinkBytes
	info.Available = true
	return info, nil
}

func linuxUserExists(username string) bool {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return false
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	prefix := username + ":"
	for s.Scan() {
		if strings.HasPrefix(s.Text(), prefix) {
			return true
		}
	}
	return false
}

func linuxExpiration(username string) (string, int, string, error) {
	if !linuxUserExists(username) {
		return "", 0, "not_found", fmt.Errorf("usuário Linux não encontrado: %s", username)
	}

	f, err := os.Open("/etc/shadow")
	if err != nil {
		return "Nunca", 9999, "active", nil
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	prefix := username + ":"
	for s.Scan() {
		line := s.Text()
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 8 || parts[7] == "" || parts[7] == "-1" {
			return "Nunca", 9999, "active", nil
		}
		daysSinceEpoch, err := strconv.ParseInt(parts[7], 10, 64)
		if err != nil || daysSinceEpoch <= 0 {
			return "Nunca", 9999, "active", nil
		}
		exp := time.Unix(daysSinceEpoch*86400, 0).Local()
		remaining := int(time.Until(exp).Hours() / 24)
		status := "active"
		if remaining < 0 {
			status = "expired"
		}
		return exp.Format("02/01/2006"), remaining, status, nil
	}
	return "", 0, "not_found", fmt.Errorf("usuário não encontrado em /etc/shadow: %s", username)
}

func detectLimit(username, dataDir string) int {
	paths := []string{
		filepath.Join(dataDir, "limits", username),
		filepath.Join("/root/checkuserdt/limits", username),
		filepath.Join("/etc/checkuser/limits", username),
		filepath.Join("/etc/SSHPlus/limite", username),
		filepath.Join("/etc/sshplus/limite", username),
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err == nil {
			if n := firstInt(string(b)); n >= 0 {
				return n
			}
		}
	}
	return 0
}

func firstInt(s string) int {
	re := regexp.MustCompile(`\d+`)
	m := re.FindString(s)
	if m == "" {
		return -1
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return -1
	}
	return n
}

func countSSHConnections(username string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-u", username)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	return bytes.Count(out, []byte("sshd"))
}

func countAllSSHConnections() int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-ef")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(string(out), "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, "sshd") && !strings.Contains(line, "grep") && !strings.Contains(line, "root") {
			count++
		}
	}
	return count
}

func (d *DeviceStore) Load() DevicesDB {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	return d.loadLocked()
}

func (d *DeviceStore) loadLocked() DevicesDB {
	db := DevicesDB{Users: map[string]map[string]int64{}}
	b, err := os.ReadFile(d.Path)
	if err != nil {
		return db
	}
	_ = json.Unmarshal(b, &db)
	if db.Users == nil {
		db.Users = map[string]map[string]int64{}
	}
	return db
}

func (d *DeviceStore) Register(username, deviceID string) error {
	if username == "" || deviceID == "" {
		return nil
	}
	d.Mu.Lock()
	defer d.Mu.Unlock()
	_ = os.MkdirAll(filepath.Dir(d.Path), 0755)
	db := d.loadLocked()
	if db.Users[username] == nil {
		db.Users[username] = map[string]int64{}
	}
	db.Users[username][deviceID] = time.Now().Unix()
	b, _ := json.MarshalIndent(db, "", "  ")
	return os.WriteFile(d.Path, b, 0644)
}

func (d *DeviceStore) Count(username string) int {
	db := d.Load()
	return len(db.Users[username])
}

func stableID(s string) int {
	h := 0
	for _, r := range s {
		h = int(r) + ((h << 5) - h)
	}
	if h < 0 {
		h = -h
	}
	return h
}

func chooseUsername(input string, client XrayClient) string {
	if client.ID != "" && strings.EqualFold(input, client.ID) {
		return client.ID
	}
	if client.Email != "" {
		return client.Email
	}
	return input
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = net.JoinHostPort("127.0.0.1", strconv.Itoa(defaultPort))
	}
	return scheme + "://" + host
}
