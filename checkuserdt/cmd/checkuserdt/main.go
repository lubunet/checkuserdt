package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const version = "1.3.0"

type App struct {
	Listen     string
	Port       int
	XrayConfig string
	XrayAPI    string
	XrayBin    string
	DataDir    string
}

type XrayConfigFile struct {
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
	Tag   string `json:"-"`
}

type TrafficInfo struct {
	UplinkBytes   int64  `json:"uplink_bytes"`
	DownlinkBytes int64  `json:"downlink_bytes"`
	TotalBytes    int64  `json:"total_bytes"`
	Available     bool   `json:"available"`
	Error         string `json:"error,omitempty"`
}

type CheckResponse struct {
	Input            string      `json:"input"`
	ID               int64       `json:"id"`
	Username         string      `json:"username"`
	UUID             string      `json:"uuid,omitempty"`
	XrayUser         string      `json:"xray_user,omitempty"`
	Mode             string      `json:"mode"`
	ExpirationDate   string      `json:"expiration_date"`
	ExpirationDays   int         `json:"expiration_days"`
	LimitConnections int         `json:"limit_connections"`
	CountConnections int         `json:"count_connections"`
	Status           string      `json:"status"`
	Traffic          TrafficInfo `json:"traffic,omitempty"`
	Version          string      `json:"version"`
	CheckedAt        string      `json:"checked_at"`
}

type ErrorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Version string `json:"version"`
}

type ShadowInfo struct {
	Found          bool
	ExpireDaysRaw  string
	ExpirationDate string
	ExpirationDays int
	Status         string
}

func main() {
	start := flag.Bool("start", false, "iniciar servidor HTTP")
	listen := flag.String("listen", "0.0.0.0", "endereco de escuta")
	port := flag.Int("port", 555, "porta HTTP")
	xrayConfig := flag.String("xray-config", "/usr/local/etc/xray/config.json", "caminho do config.json do Xray")
	xrayAPI := flag.String("xray-api", "127.0.0.1:1085", "endereco da API gRPC do Xray")
	xrayBin := flag.String("xray-bin", "/usr/local/bin/xray", "caminho do binario xray")
	dataDir := flag.String("data-dir", "/root/checkuserdt/data", "diretorio de dados")
	showVersion := flag.Bool("version", false, "mostrar versao")
	flag.Parse()

	if *showVersion {
		fmt.Println("checkuserdt", version)
		return
	}

	if !*start {
		fmt.Println("checkuserdt", version)
		fmt.Println("Use com systemd ou: checkuserdt --start --port 555")
		return
	}

	app := &App{Listen: *listen, Port: *port, XrayConfig: *xrayConfig, XrayAPI: *xrayAPI, XrayBin: *xrayBin, DataDir: *dataDir}
	if err := os.MkdirAll(app.DataDir, 0700); err != nil {
		log.Printf("aviso: nao foi possivel criar data-dir: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleRootOrDirect)
	mux.HandleFunc("/health", app.handleHealth)
	mux.HandleFunc("/check/", app.handleCheck)
	mux.HandleFunc("/details/", app.handleCheck)
	mux.HandleFunc("/stats/", app.handleStats)
	mux.HandleFunc("/users", app.handleUsers)
	mux.HandleFunc("/count", app.handleCount)

	addr := fmt.Sprintf("%s:%d", app.Listen, app.Port)
	log.Printf("checkuserdt v%s ouvindo em http://%s", version, addr)
	log.Printf("config Xray: %s | API Xray: %s", app.XrayConfig, app.XrayAPI)
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) handleRootOrDirect(w http.ResponseWriter, r *http.Request) {
	// Suporta o uso igual a muitos checkuser antigos: o app recebe apenas a URL base
	// e envia o usuario/uuid por query ou acrescenta no caminho.
	input := firstNonEmpty(
		r.URL.Query().Get("username"),
		r.URL.Query().Get("user"),
		r.URL.Query().Get("usuario"),
		r.URL.Query().Get("uuid"),
		r.URL.Query().Get("id"),
	)
	if input == "" {
		path := strings.Trim(r.URL.Path, "/")
		if path != "" && path != "favicon.ico" {
			input = path
		}
	}
	if input == "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "online", "service": "checkuserdt", "version": version})
		return
	}
	a.writeCheckForInput(w, input)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "checkuserdt", "version": version, "port": a.Port, "time": time.Now().Format(time.RFC3339)})
}

func (a *App) handleCheck(w http.ResponseWriter, r *http.Request) {
	input := cleanPathValue(r.URL.Path)
	if input == "" {
		input = firstNonEmpty(r.URL.Query().Get("username"), r.URL.Query().Get("user"), r.URL.Query().Get("usuario"), r.URL.Query().Get("uuid"), r.URL.Query().Get("id"))
	}
	if input == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{"error", "usuario ou uuid nao informado", version})
		return
	}
	a.writeCheckForInput(w, input)
}

func (a *App) writeCheckForInput(w http.ResponseWriter, input string) {
	resp, err := a.resolveInput(input, true)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{"not_found", err.Error(), version})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	input := cleanPathValue(r.URL.Path)
	if input == "" {
		input = firstNonEmpty(r.URL.Query().Get("username"), r.URL.Query().Get("user"), r.URL.Query().Get("usuario"), r.URL.Query().Get("uuid"), r.URL.Query().Get("id"))
	}
	if input == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{"error", "usuario ou uuid nao informado", version})
		return
	}
	resp, err := a.resolveInput(input, true)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{"not_found", err.Error(), version})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"input": resp.Input, "username": resp.Username, "uuid": resp.UUID, "xray_user": resp.XrayUser, "traffic": resp.Traffic, "version": version})
}

func (a *App) handleUsers(w http.ResponseWriter, r *http.Request) {
	clients, _ := a.loadXrayClients()
	type item struct {
		UUID             string      `json:"uuid"`
		XrayUser         string      `json:"xray_user"`
		ExpirationDate   string      `json:"expiration_date"`
		ExpirationDays   int         `json:"expiration_days"`
		Status           string      `json:"status"`
		LimitConnections int         `json:"limit_connections"`
		CountConnections int         `json:"count_connections"`
		Traffic          TrafficInfo `json:"traffic,omitempty"`
	}
	items := make([]item, 0, len(clients))
	for _, c := range clients {
		shadow := getShadowInfo(c.Email)
		items = append(items, item{UUID: c.ID, XrayUser: c.Email, ExpirationDate: shadow.ExpirationDate, ExpirationDays: shadow.ExpirationDays, Status: shadow.Status, LimitConnections: readLimit(a.DataDir, c.Email), CountConnections: countUserProcesses(c.Email), Traffic: a.getTraffic(c.Email)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "users": items, "version": version})
}

func (a *App) handleCount(w http.ResponseWriter, r *http.Request) {
	clients, _ := a.loadXrayClients()
	writeJSON(w, http.StatusOK, map[string]any{"count": len(clients), "version": version})
}

func (a *App) resolveInput(input string, withTraffic bool) (*CheckResponse, error) {
	input = strings.TrimSpace(input)
	clients, _ := a.loadXrayClients()
	byUUID := map[string]XrayClient{}
	byUser := map[string]XrayClient{}
	for _, c := range clients {
		if c.ID != "" {
			byUUID[strings.ToLower(c.ID)] = c
		}
		if c.Email != "" {
			byUser[c.Email] = c
		}
	}

	mode := "linux_user"
	linuxUser := input
	uuid := ""
	xrayUser := ""
	if c, ok := byUUID[strings.ToLower(input)]; ok {
		mode = "xray_uuid"
		linuxUser = c.Email
		xrayUser = c.Email
		uuid = c.ID
	} else if c, ok := byUser[input]; ok {
		mode = "linux_user"
		linuxUser = input
		xrayUser = c.Email
		uuid = c.ID
	}

	if linuxUser == "" {
		return nil, fmt.Errorf("uuid encontrado, mas email/usuario vazio no config do Xray")
	}
	if !safeUsername(linuxUser) || !userExists(linuxUser) {
		return nil, fmt.Errorf("usuario %q nao existe no Linux ou nao foi encontrado", linuxUser)
	}
	shadow := getShadowInfo(linuxUser)
	if !shadow.Found {
		return nil, fmt.Errorf("nao foi possivel ler expiracao de %q em /etc/shadow; execute o servico como root", linuxUser)
	}

	traffic := TrafficInfo{}
	if withTraffic {
		traffic = a.getTraffic(linuxUser)
	}
	usernameField := input
	if mode == "linux_user" {
		usernameField = linuxUser
	}

	return &CheckResponse{Input: input, ID: getUserID(linuxUser), Username: usernameField, UUID: uuid, XrayUser: xrayUser, Mode: mode, ExpirationDate: shadow.ExpirationDate, ExpirationDays: shadow.ExpirationDays, LimitConnections: readLimit(a.DataDir, linuxUser), CountConnections: countUserProcesses(linuxUser), Status: shadow.Status, Traffic: traffic, Version: version, CheckedAt: time.Now().Format(time.RFC3339)}, nil
}

func (a *App) loadXrayClients() ([]XrayClient, error) {
	b, err := os.ReadFile(a.XrayConfig)
	if err != nil {
		return nil, err
	}
	var cfg XrayConfigFile
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	var out []XrayClient
	for _, in := range cfg.Inbounds {
		for _, c := range in.Settings.Clients {
			c.Tag = in.Tag
			if c.ID != "" || c.Email != "" {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func (a *App) getTraffic(email string) TrafficInfo {
	if email == "" {
		return TrafficInfo{Available: false, Error: "usuario/email vazio"}
	}
	if _, err := os.Stat(a.XrayBin); err != nil {
		return TrafficInfo{Available: false, Error: "xray binario nao encontrado"}
	}
	pattern := fmt.Sprintf("user>>>%s>>>traffic", email)
	cmd := exec.Command(a.XrayBin, "api", "statsquery", "--server="+a.XrayAPI, "-pattern", pattern, "-reset=false")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return TrafficInfo{Available: false, Error: msg}
	}
	up, down := parseXrayStats(stdout.String(), email)
	return TrafficInfo{UplinkBytes: up, DownlinkBytes: down, TotalBytes: up + down, Available: true}
}

func parseXrayStats(s, email string) (int64, int64) {
	var up, down int64
	lines := strings.Split(s, "\n")
	var lastName string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.Contains(line, "name:") && strings.Contains(line, "traffic") {
			lastName = line
		}
		if strings.Contains(line, "value:") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if strings.TrimRight(f, ":") == "value" && i+1 < len(fields) {
					v, _ := strconv.ParseInt(strings.Trim(fields[i+1], "\""), 10, 64)
					if strings.Contains(lastName, "uplink") {
						up += v
					}
					if strings.Contains(lastName, "downlink") {
						down += v
					}
				}
			}
		}
	}
	if up == 0 && down == 0 {
		for _, token := range []string{"uplink", "downlink"} {
			idx := strings.Index(s, fmt.Sprintf("user>>>%s>>>traffic>>>%s", email, token))
			if idx >= 0 {
				part := s[idx:]
				if v, ok := extractFirstValue(part); ok {
					if token == "uplink" {
						up = v
					} else {
						down = v
					}
				}
			}
		}
	}
	return up, down
}

func extractFirstValue(s string) (int64, bool) {
	idx := strings.Index(s, "value:")
	if idx < 0 {
		return 0, false
	}
	fields := strings.Fields(s[idx:])
	if len(fields) < 2 {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.Trim(fields[1], "\""), 10, 64)
	return v, err == nil
}

func getShadowInfo(username string) ShadowInfo {
	file, err := os.Open("/etc/shadow")
	if err != nil {
		return ShadowInfo{Found: false, ExpirationDate: "unknown", ExpirationDays: 0, Status: "unknown"}
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) < 8 || parts[0] != username {
			continue
		}
		expireRaw := parts[7]
		if expireRaw == "" || expireRaw == "-1" {
			return ShadowInfo{Found: true, ExpireDaysRaw: expireRaw, ExpirationDate: "never", ExpirationDays: 99999, Status: "active"}
		}
		expireDays, err := strconv.ParseInt(expireRaw, 10, 64)
		if err != nil || expireDays <= 0 {
			return ShadowInfo{Found: true, ExpireDaysRaw: expireRaw, ExpirationDate: "unknown", ExpirationDays: 0, Status: "unknown"}
		}
		nowDays := time.Now().Unix() / 86400
		remaining := int(expireDays - nowDays)
		expDate := time.Unix(expireDays*86400, 0).Format("02/01/2006")
		status := "active"
		if remaining < 0 {
			status = "expired"
		}
		return ShadowInfo{Found: true, ExpireDaysRaw: expireRaw, ExpirationDate: expDate, ExpirationDays: remaining, Status: status}
	}
	return ShadowInfo{Found: false, ExpirationDate: "not_found", ExpirationDays: 0, Status: "not_found"}
}

func safeUsername(username string) bool {
	return username != "" && !strings.Contains(username, "/") && !strings.Contains(username, "\x00") && !strings.Contains(username, "..")
}

func userExists(username string) bool {
	if !safeUsername(username) {
		return false
	}
	_, err := user.Lookup(username)
	if err == nil {
		return true
	}
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

func getUserID(username string) int64 {
	u, err := user.Lookup(username)
	if err != nil {
		return 0
	}
	uid, _ := strconv.ParseInt(u.Uid, 10, 64)
	return uid
}

func countUserProcesses(username string) int {
	if username == "" {
		return 0
	}
	cmd := exec.Command("ps", "-u", username, "-o", "pid=")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func readLimit(dataDir, username string) int {
	paths := []string{filepath.Join(dataDir, "limits", username), filepath.Join(filepath.Dir(dataDir), "limits", username)}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err == nil {
			v, _ := strconv.Atoi(strings.TrimSpace(string(b)))
			return v
		}
	}
	return 0
}

func cleanPathValue(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
