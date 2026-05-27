package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const version = "1.2.0"

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
		fmt.Println("Uso:")
		fmt.Printf("  %s --start --listen 0.0.0.0 --port 555 --xray-config /usr/local/etc/xray/config.json\n", os.Args[0])
		return
	}

	app := &App{
		Listen:     *listen,
		Port:       *port,
		XrayConfig: *xrayConfig,
		XrayAPI:    *xrayAPI,
		XrayBin:    *xrayBin,
		DataDir:    *dataDir,
	}

	if err := os.MkdirAll(app.DataDir, 0700); err != nil {
		log.Printf("aviso: nao foi possivel criar data-dir: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleIndex)
	mux.HandleFunc("/health", app.handleHealth)
	mux.HandleFunc("/check/", app.handleCheck)
	mux.HandleFunc("/details/", app.handleCheck)
	mux.HandleFunc("/stats/", app.handleStats)
	mux.HandleFunc("/users", app.handleUsers)
	mux.HandleFunc("/count", app.handleCount)
	mux.HandleFunc("/panel", app.handlePanel)

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

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "checkuserdt",
		"version": version,
		"routes":  []string{"/health", "/check/USUARIO_OU_UUID", "/details/USUARIO_OU_UUID", "/stats/USUARIO_OU_UUID", "/users", "/count", "/panel"},
	})
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"service":     "checkuserdt",
		"version":     version,
		"port":        a.Port,
		"xray_config": a.XrayConfig,
		"time":        time.Now().Format(time.RFC3339),
	})
}

func (a *App) handleCheck(w http.ResponseWriter, r *http.Request) {
	input := cleanPathValue(r.URL.Path)
	if input == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{"error", "informe usuario ou uuid no final da URL", version})
		return
	}

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
		writeJSON(w, http.StatusBadRequest, ErrorResponse{"error", "informe usuario ou uuid no final da URL", version})
		return
	}
	resp, err := a.resolveInput(input, true)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{"not_found", err.Error(), version})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"input":     resp.Input,
		"username":  resp.Username,
		"uuid":      resp.UUID,
		"xray_user": resp.XrayUser,
		"traffic":   resp.Traffic,
		"version":   version,
	})
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
		items = append(items, item{
			UUID:             c.ID,
			XrayUser:         c.Email,
			ExpirationDate:   shadow.ExpirationDate,
			ExpirationDays:   shadow.ExpirationDays,
			Status:           shadow.Status,
			LimitConnections: readLimit(a.DataDir, c.Email),
			CountConnections: countUserProcesses(c.Email),
			Traffic:          a.getTraffic(c.Email),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "users": items, "version": version})
}

func (a *App) handleCount(w http.ResponseWriter, r *http.Request) {
	clients, _ := a.loadXrayClients()
	writeJSON(w, http.StatusOK, map[string]any{"count": len(clients), "version": version})
}

func (a *App) handlePanel(w http.ResponseWriter, r *http.Request) {
	clients, _ := a.loadXrayClients()
	type row struct {
		UUID           string
		User           string
		ExpirationDate string
		ExpirationDays int
		Status         string
		Up             string
		Down           string
		Total          string
	}
	rows := make([]row, 0, len(clients))
	for _, c := range clients {
		shadow := getShadowInfo(c.Email)
		traffic := a.getTraffic(c.Email)
		rows = append(rows, row{
			UUID:           c.ID,
			User:           c.Email,
			ExpirationDate: shadow.ExpirationDate,
			ExpirationDays: shadow.ExpirationDays,
			Status:         shadow.Status,
			Up:             humanBytes(traffic.UplinkBytes),
			Down:           humanBytes(traffic.DownlinkBytes),
			Total:          humanBytes(traffic.TotalBytes),
		})
	}
	data := map[string]any{"Rows": rows, "Version": version, "Now": time.Now().Format("02/01/2006 15:04:05")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = panelTpl.Execute(w, data)
}

func (a *App) resolveInput(input string, withTraffic bool) (*CheckResponse, error) {
	clients, _ := a.loadXrayClients()
	byUUID := map[string]XrayClient{}
	byUser := map[string]XrayClient{}
	for _, c := range clients {
		byUUID[strings.ToLower(c.ID)] = c
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

	if !userExists(linuxUser) {
		return nil, fmt.Errorf("usuario %q nao existe no Linux ou nao foi encontrado", linuxUser)
	}

	shadow := getShadowInfo(linuxUser)
	if !shadow.Found {
		return nil, fmt.Errorf("nao foi possivel ler expiracao de %q em /etc/shadow; execute o servico como root", linuxUser)
	}

	uid := getUserID(linuxUser)
	traffic := TrafficInfo{}
	if withTraffic {
		traffic = a.getTraffic(linuxUser)
	}

	usernameField := input
	if mode == "linux_user" {
		usernameField = linuxUser
	}

	return &CheckResponse{
		Input:            input,
		ID:               uid,
		Username:         usernameField,
		UUID:             uuid,
		XrayUser:         xrayUser,
		Mode:             mode,
		ExpirationDate:   shadow.ExpirationDate,
		ExpirationDays:   shadow.ExpirationDays,
		LimitConnections: readLimit(a.DataDir, linuxUser),
		CountConnections: countUserProcesses(linuxUser),
		Status:           shadow.Status,
		Traffic:          traffic,
		Version:          version,
		CheckedAt:        time.Now().Format(time.RFC3339),
	}, nil
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

	// Fallback para saidas em uma unica linha.
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

func userExists(username string) bool {
	if username == "" || strings.Contains(username, "/") || strings.Contains(username, "\x00") {
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
	paths := []string{
		filepath.Join(dataDir, "limits", username),
		filepath.Join(filepath.Dir(dataDir), "limits", username),
	}
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
	v := strings.TrimSpace(parts[len(parts)-1])
	v = strings.Trim(v, " \t\n\r")
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func humanBytes(v int64) string {
	if v < 1024 {
		return fmt.Sprintf("%d B", v)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	f := float64(v)
	for _, u := range units {
		f /= 1024
		if f < 1024 {
			return fmt.Sprintf("%.2f %s", f, u)
		}
	}
	return fmt.Sprintf("%d B", v)
}

func localIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip != nil {
				return ip.String()
			}
		}
	}
	return ""
}

var panelTpl = template.Must(template.New("panel").Parse(`<!doctype html>
<html lang="pt-br">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>CheckUserDT</title>
<style>
body{font-family:Arial,sans-serif;background:#0b1020;color:#f8fafc;margin:0;padding:28px}.card{background:#111936;border:1px solid #26345f;border-radius:16px;padding:20px;box-shadow:0 20px 60px #0005}h1{margin:0 0 6px}.muted{color:#9ca3af}table{width:100%;border-collapse:collapse;margin-top:18px}th,td{padding:10px;border-bottom:1px solid #26345f;text-align:left;font-size:14px}th{color:#cbd5e1}.active{color:#22c55e;font-weight:bold}.expired{color:#ef4444;font-weight:bold}.pill{display:inline-block;background:#172554;border:1px solid #1e40af;border-radius:999px;padding:4px 10px;font-size:12px}.uuid{font-family:monospace;font-size:12px;color:#c4b5fd}.top{display:flex;justify-content:space-between;gap:12px;align-items:center;flex-wrap:wrap}.btn{color:#fff;background:#2563eb;text-decoration:none;padding:8px 12px;border-radius:10px}</style>
</head>
<body>
<div class="card">
  <div class="top">
    <div><h1>CheckUserDT</h1><div class="muted">Versão {{.Version}} · atualizado em {{.Now}}</div></div>
    <a class="btn" href="/users">JSON /users</a>
  </div>
  <table>
    <thead><tr><th>Usuário</th><th>UUID</th><th>Validade</th><th>Dias</th><th>Status</th><th>Upload</th><th>Download</th><th>Total</th></tr></thead>
    <tbody>
      {{range .Rows}}
      <tr>
        <td>{{.User}}</td>
        <td class="uuid">{{.UUID}}</td>
        <td>{{.ExpirationDate}}</td>
        <td>{{.ExpirationDays}}</td>
        <td><span class="{{.Status}}">{{.Status}}</span></td>
        <td>{{.Up}}</td>
        <td>{{.Down}}</td>
        <td><span class="pill">{{.Total}}</span></td>
      </tr>
      {{else}}
      <tr><td colspan="8" class="muted">Nenhum cliente Xray encontrado em config.json.</td></tr>
      {{end}}
    </tbody>
  </table>
</div>
</body>
</html>`))

var errNoop = errors.New("noop")
