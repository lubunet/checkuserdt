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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPort       = 555
	defaultXrayConfig = "/usr/local/etc/xray/config.json"
	defaultDataDir    = "/root/checkuserdt/data"
	onlineTTL         = 10 * time.Minute
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
	Key            string `json:"key"`
	Source         string `json:"source,omitempty"`
	Username       string `json:"username"`
	UUID           string `json:"uuid,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	DTunnelID      string `json:"dtunnel_id,omitempty"`
	VPNState       string `json:"vpn_state,omitempty"`
	ConfigID       string `json:"config_id,omitempty"`
	ConfigName     string `json:"config_name,omitempty"`
	ConfigCategory string `json:"config_category,omitempty"`
	LocalIP        string `json:"local_ip,omitempty"`
	PublicIP       string `json:"public_ip,omitempty"`
	NetworkName    string `json:"network_name,omitempty"`
	NetworkType    string `json:"network_type,omitempty"`
	NetworkExtra   string `json:"network_extra_info,omitempty"`
	Operator       string `json:"operator,omitempty"`
	Carrier        string `json:"carrier,omitempty"`
	AppVersion     string `json:"app_version,omitempty"`
	AndroidVer     string `json:"android_version,omitempty"`
	DownloadBytes  int64  `json:"download_bytes,omitempty"`
	UploadBytes    int64  `json:"upload_bytes,omitempty"`
	ConnectedAt    int64  `json:"connected_at,omitempty"`
	FirstSeen      int64  `json:"first_seen"`
	LastSeen       int64  `json:"last_seen"`
	Category       string `json:"category"`
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
	mux.HandleFunc("/telemetry/pixel", app.withCORS(app.handleHeartbeat))
	mux.HandleFunc("/hb", app.withCORS(app.handleHeartbeat))
	mux.HandleFunc("/telemetry/disconnect", app.withCORS(app.handleDisconnect))
	mux.HandleFunc("/online/disconnect", app.withCORS(app.handleDisconnect))
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
	a.writeCheck(w, r, input)
}

func (a *App) handleCheckPath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "message": "informe usuario ou uuid"})
		return
	}
	input := strings.TrimSpace(parts[1])
	a.writeCheck(w, r, input)
}

func (a *App) writeCheck(w http.ResponseWriter, r *http.Request, input string) {
	info, err := a.ResolveUser(input)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"input": input, "status": "not_found", "message": err.Error()})
		return
	}
	a.TouchCheckOnline(r, info)
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
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "message": "use GET ou POST"})
		return
	}
	s, err := a.parseOnlineSessionFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "message": err.Error()})
		return
	}

	// Se o WebView avisar que desconectou, removemos da lista de online imediatamente.
	if isOfflineVPNState(s.VPNState) {
		a.Store.Delete(s.Key)
		// Se ainda houver um registro antigo vindo apenas do /check com o mesmo IP/usuario,
		// limpamos tambem para a opcao 5 nao continuar mostrando usuario desconectado.
		a.Store.DeleteRelated(s)
		writeJSONOrPixel(w, r, http.StatusOK, map[string]any{"status": "offline", "session": s})
		return
	}

	now := time.Now().Unix()
	if s.FirstSeen == 0 {
		s.FirstSeen = now
	}
	s.LastSeen = now
	s.Source = firstNonEmpty(s.Source, "heartbeat")
	s.Category = firstNonEmpty(s.ConfigCategory, s.Category, detectCategory(s))
	s.Key = makeSessionKey(s)
	a.Store.Upsert(s)
	writeJSONOrPixel(w, r, http.StatusOK, map[string]any{"status": "ok", "session": s})
}

func (a *App) parseOnlineSessionFromRequest(r *http.Request) (OnlineSession, error) {
	var s OnlineSession
	if r.Method == http.MethodPost {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1024*128))
		_ = json.Unmarshal(body, &s)
	}
	q := r.URL.Query()
	fillIfEmpty(&s.Username, firstNonEmpty(q.Get("username"), q.Get("user")))
	fillIfEmpty(&s.UUID, firstNonEmpty(q.Get("uuid"), q.Get("id")))
	fillIfEmpty(&s.DeviceID, firstNonEmpty(q.Get("device_id"), q.Get("deviceId"), q.Get("device")))
	fillIfEmpty(&s.VPNState, firstNonEmpty(q.Get("vpn_state"), q.Get("vpnState"), q.Get("state")))
	fillIfEmpty(&s.ConfigID, firstNonEmpty(q.Get("config_id"), q.Get("configId")))
	fillIfEmpty(&s.ConfigName, firstNonEmpty(q.Get("config_name"), q.Get("configName"), q.Get("config")))
	fillIfEmpty(&s.ConfigCategory, firstNonEmpty(q.Get("config_category"), q.Get("category"), q.Get("cat")))
	fillIfEmpty(&s.LocalIP, firstNonEmpty(q.Get("local_ip"), q.Get("localIp")))
	fillIfEmpty(&s.PublicIP, firstNonEmpty(q.Get("public_ip"), q.Get("publicIp")))
	fillIfEmpty(&s.NetworkName, firstNonEmpty(q.Get("network_name"), q.Get("networkName"), q.Get("network")))
	fillIfEmpty(&s.NetworkType, firstNonEmpty(q.Get("network_type"), q.Get("networkType")))
	fillIfEmpty(&s.NetworkExtra, firstNonEmpty(q.Get("network_extra_info"), q.Get("networkExtra")))
	fillIfEmpty(&s.Operator, q.Get("operator"))
	fillIfEmpty(&s.Carrier, q.Get("carrier"))
	fillIfEmpty(&s.DTunnelID, firstNonEmpty(q.Get("dtunnel_id"), q.Get("dtunnelId")))
	fillIfEmpty(&s.AppVersion, firstNonEmpty(q.Get("app_version"), q.Get("appVersion")))
	fillIfEmpty(&s.AndroidVer, firstNonEmpty(q.Get("android_version"), q.Get("androidVersion")))
	if v, err := strconv.ParseInt(firstNonEmpty(q.Get("download_bytes"), q.Get("downloadBytes")), 10, 64); err == nil && v > 0 {
		s.DownloadBytes = v
	}
	if v, err := strconv.ParseInt(firstNonEmpty(q.Get("upload_bytes"), q.Get("uploadBytes")), 10, 64); err == nil && v > 0 {
		s.UploadBytes = v
	}
	if v, err := strconv.ParseInt(firstNonEmpty(q.Get("connected_at"), q.Get("connectedAt")), 10, 64); err == nil && v > 0 {
		s.ConnectedAt = v
	}

	if s.PublicIP == "" {
		s.PublicIP = clientIP(r)
	}
	if s.VPNState == "" {
		s.VPNState = "CONNECTED"
	}

	if s.UUID != "" && s.Username == "" {
		if info, err := a.ResolveUser(s.UUID); err == nil {
			s.Username = info.Username
		}
	}
	// Alguns WebViews nao devolvem DtUsername/DtUuid no HTML, mas o /check do app
	// chega antes no servidor. Nesse caso vinculamos o heartbeat pelo IP publico recente.
	if s.Username == "" {
		if recent, ok := a.Store.FindRecentCheckByPublicIP(s.PublicIP, 15*time.Minute); ok {
			s.Username = recent.Username
			if s.UUID == "" {
				s.UUID = recent.UUID
			}
		}
	}
	// Ultimo fallback: pelo menos mostramos o device online. Assim fica facil saber
	// que o heartbeat chegou mesmo se o app nao expor usuario/uuid na WebView.
	if s.Username == "" && s.DeviceID != "" {
		s.Username = "device-" + shortTail(s.DeviceID, 6)
	}
	if s.Username == "" && s.UUID == "" && s.DeviceID == "" {
		return s, fmt.Errorf("informe username, uuid ou device_id")
	}
	s.Category = firstNonEmpty(s.ConfigCategory, s.Category, detectCategory(s))
	s.Key = makeSessionKey(s)
	return s, nil
}

func (a *App) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"status": "error", "message": "use GET ou POST"})
		return
	}
	s, err := a.parseOnlineSessionFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "message": err.Error()})
		return
	}
	s.VPNState = firstNonEmpty(s.VPNState, "DISCONNECTED")
	s.Key = makeSessionKey(s)
	a.Store.Delete(s.Key)
	a.Store.DeleteRelated(s)
	writeJSONOrPixel(w, r, http.StatusOK, map[string]any{"status": "offline", "session": s})
}

func (a *App) handleOnline(w http.ResponseWriter, r *http.Request) {
	items := a.Store.List(r.URL.Query().Get("all") != "1")
	writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "ttl_seconds": int(onlineTTL.Seconds()), "users": items})
}

func (a *App) handleOnlineCategories(w http.ResponseWriter, r *http.Request) {
	counts := map[string]int{}
	for _, s := range a.Store.List(true) {
		cat := strings.TrimSpace(s.Category)
		if cat == "" {
			cat = "Outros"
		}
		counts[cat]++
	}

	// Primeiro mostramos as categorias mais comuns, depois qualquer categoria real
	// que venha do app/DTunnel via config_category. A v6 tinha um bug aqui: se a
	// categoria fosse "CheckUser", ela ficava salva, mas nao aparecia no menu.
	preferred := []string{"Vivo", "Tim", "Claro", "Oi", "Wi-Fi", "Dados moveis", "CheckUser", "Outros"}
	order := make([]string, 0, len(counts))
	seen := map[string]bool{}
	for _, name := range preferred {
		if counts[name] > 0 {
			order = append(order, name)
			seen[name] = true
		}
	}
	var extra []string
	for name := range counts {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	order = append(order, extra...)

	if r.URL.Query().Get("format") == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, name := range order {
			fmt.Fprintf(w, "%s|%d\n", name, counts[name])
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
			if s.Source != "" {
				fmt.Fprintf(w, "    Fonte: %s\n", s.Source)
			}
			if s.UUID != "" {
				fmt.Fprintf(w, "    UUID: %s\n", s.UUID)
			}
			if s.DeviceID != "" {
				fmt.Fprintf(w, "    Device: %s\n", s.DeviceID)
			}
			if s.ConfigName != "" || s.ConfigID != "" {
				fmt.Fprintf(w, "    Config: %s %s\n", s.ConfigName, s.ConfigID)
			}
			if s.ConfigCategory != "" {
				fmt.Fprintf(w, "    Categoria do app: %s\n", s.ConfigCategory)
			}
			if s.LocalIP != "" {
				fmt.Fprintf(w, "    IP local: %s\n", s.LocalIP)
			}
			if s.PublicIP != "" {
				fmt.Fprintf(w, "    IP publico: %s\n", s.PublicIP)
			}
			if s.NetworkName != "" || s.NetworkType != "" || s.NetworkExtra != "" {
				fmt.Fprintf(w, "    Rede: %s %s %s\n", s.NetworkName, s.NetworkType, s.NetworkExtra)
			}
			if s.Operator != "" || s.Carrier != "" {
				fmt.Fprintf(w, "    Operadora: %s %s\n", s.Operator, s.Carrier)
			}
			if s.VPNState != "" {
				fmt.Fprintf(w, "    VPN: %s\n", s.VPNState)
			}
			if s.DownloadBytes > 0 || s.UploadBytes > 0 {
				fmt.Fprintf(w, "    Trafego: down=%d bytes | up=%d bytes\n", s.DownloadBytes, s.UploadBytes)
			}
			if s.ConnectedAt > 0 {
				fmt.Fprintf(w, "    Conectado desde: %s\n", formatMillisOrUnix(s.ConnectedAt))
			}
			fmt.Fprintf(w, "    Ultimo sinal: %s\n\n", time.Unix(s.LastSeen, 0).Format("02/01/2006 15:04:05"))
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"category": cat, "count": len(rows), "users": rows})
}

func (a *App) TouchCheckOnline(r *http.Request, info *UserInfo) {
	if info == nil || info.Username == "" {
		return
	}
	now := time.Now().Unix()
	s := OnlineSession{
		Username:  info.Username,
		UUID:      info.UUID,
		PublicIP:  clientIP(r),
		VPNState:  "CHECKED",
		Category:  "CheckUser",
		Source:    "checkuser",
		FirstSeen: now,
		LastSeen:  now,
	}
	if r != nil {
		q := r.URL.Query()
		fillIfEmpty(&s.DeviceID, firstNonEmpty(q.Get("device_id"), q.Get("deviceId")))
		fillIfEmpty(&s.NetworkName, firstNonEmpty(q.Get("network_name"), q.Get("networkName"), q.Get("network")))
		fillIfEmpty(&s.NetworkType, firstNonEmpty(q.Get("network_type"), q.Get("networkType")))
		fillIfEmpty(&s.NetworkExtra, firstNonEmpty(q.Get("network_extra_info"), q.Get("networkExtra")))
		fillIfEmpty(&s.Operator, q.Get("operator"))
		fillIfEmpty(&s.Carrier, q.Get("carrier"))
		fillIfEmpty(&s.LocalIP, firstNonEmpty(q.Get("local_ip"), q.Get("localIp")))
		fillIfEmpty(&s.DTunnelID, firstNonEmpty(q.Get("dtunnel_id"), q.Get("dtunnelId")))
		fillIfEmpty(&s.AppVersion, firstNonEmpty(q.Get("app_version"), q.Get("appVersion")))
		fillIfEmpty(&s.ConfigID, firstNonEmpty(q.Get("config_id"), q.Get("configId")))
		fillIfEmpty(&s.ConfigName, firstNonEmpty(q.Get("config_name"), q.Get("configName"), q.Get("config")))
		fillIfEmpty(&s.ConfigCategory, firstNonEmpty(q.Get("config_category"), q.Get("category"), q.Get("cat")))
		if v := firstNonEmpty(q.Get("vpn_state"), q.Get("vpnState"), q.Get("state")); v != "" {
			s.VPNState = v
		}
		if v, err := strconv.ParseInt(firstNonEmpty(q.Get("download_bytes"), q.Get("downloadBytes")), 10, 64); err == nil && v > 0 {
			s.DownloadBytes = v
		}
		if v, err := strconv.ParseInt(firstNonEmpty(q.Get("upload_bytes"), q.Get("uploadBytes")), 10, 64); err == nil && v > 0 {
			s.UploadBytes = v
		}
		if v, err := strconv.ParseInt(firstNonEmpty(q.Get("connected_at"), q.Get("connectedAt")), 10, 64); err == nil && v > 0 {
			s.ConnectedAt = v
		}
		if source := firstNonEmpty(q.Get("source"), q.Get("src")); source != "" {
			s.Source = source
		} else if s.ConfigName != "" || s.LocalIP != "" || s.NetworkName != "" {
			s.Source = "webview-check"
		}
		if s.ConfigCategory != "" {
			s.Category = s.ConfigCategory
		} else if s.NetworkName != "" || s.NetworkType != "" || s.Operator != "" || s.Carrier != "" {
			s.Category = detectCategory(s)
		}
	}
	s.Key = makeSessionKey(s)
	a.Store.Upsert(s)
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
	if old, ok := s.Items[item.Key]; ok {
		item = mergeSession(old, item)
	}
	s.Items[item.Key] = item
	_ = s.Save()
}

func (s *OnlineStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == "" {
		return
	}
	delete(s.Items, key)
	_ = s.Save()
}

func (s *OnlineStore) DeleteRelated(item OnlineSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.Items {
		if item.Username != "" && v.Username == item.Username {
			delete(s.Items, k)
			continue
		}
		if item.UUID != "" && v.UUID != "" && strings.EqualFold(v.UUID, item.UUID) {
			delete(s.Items, k)
			continue
		}
		if item.DeviceID != "" && v.DeviceID != "" && v.DeviceID == item.DeviceID {
			delete(s.Items, k)
			continue
		}
		if item.PublicIP != "" && v.PublicIP == item.PublicIP && v.Source == "checkuser" {
			delete(s.Items, k)
		}
	}
	_ = s.Save()
}

func (s *OnlineStore) FindRecentCheckByPublicIP(ip string, maxAge time.Duration) (OnlineSession, bool) {
	if strings.TrimSpace(ip) == "" {
		return OnlineSession{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	maxAgeSec := int64(maxAge.Seconds())
	var best OnlineSession
	for _, v := range s.Items {
		if v.PublicIP != ip || v.Username == "" {
			continue
		}
		if maxAgeSec > 0 && now-v.LastSeen > maxAgeSec {
			continue
		}
		if best.LastSeen == 0 || v.LastSeen > best.LastSeen {
			best = v
		}
	}
	return best, best.Username != ""
}

func mergeSession(old, item OnlineSession) OnlineSession {
	if item.FirstSeen == 0 || item.FirstSeen == item.LastSeen {
		item.FirstSeen = old.FirstSeen
	}
	if item.Key == "" {
		item.Key = old.Key
	}
	if item.Source == "" {
		item.Source = old.Source
	}
	if item.Username == "" {
		item.Username = old.Username
	}
	if item.UUID == "" {
		item.UUID = old.UUID
	}
	if item.DeviceID == "" {
		item.DeviceID = old.DeviceID
	}
	if item.DTunnelID == "" {
		item.DTunnelID = old.DTunnelID
	}
	if item.VPNState == "" || item.VPNState == "CHECKED" {
		if old.VPNState != "" && old.VPNState != "CHECKED" {
			item.VPNState = old.VPNState
		}
	}
	if item.ConfigID == "" {
		item.ConfigID = old.ConfigID
	}
	if item.ConfigName == "" {
		item.ConfigName = old.ConfigName
	}
	if item.ConfigCategory == "" {
		item.ConfigCategory = old.ConfigCategory
	}
	if item.LocalIP == "" {
		item.LocalIP = old.LocalIP
	}
	if item.PublicIP == "" {
		item.PublicIP = old.PublicIP
	}
	if item.NetworkName == "" {
		item.NetworkName = old.NetworkName
	}
	if item.NetworkType == "" {
		item.NetworkType = old.NetworkType
	}
	if item.NetworkExtra == "" {
		item.NetworkExtra = old.NetworkExtra
	}
	if item.Operator == "" {
		item.Operator = old.Operator
	}
	if item.Carrier == "" {
		item.Carrier = old.Carrier
	}
	if item.AppVersion == "" {
		item.AppVersion = old.AppVersion
	}
	if item.AndroidVer == "" {
		item.AndroidVer = old.AndroidVer
	}
	if item.DownloadBytes == 0 {
		item.DownloadBytes = old.DownloadBytes
	}
	if item.UploadBytes == 0 {
		item.UploadBytes = old.UploadBytes
	}
	if item.ConnectedAt == 0 {
		item.ConnectedAt = old.ConnectedAt
	}
	if item.Category == "" || item.Category == "CheckUser" {
		if old.ConfigCategory != "" {
			item.Category = old.ConfigCategory
		} else if old.Category != "" && old.Category != "CheckUser" {
			item.Category = old.Category
		}
	}
	return item
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
		// O campo de expiracao do /etc/shadow e uma DATA em dias desde 1970-01-01,
		// nao um horario exato. Se converter direto para time.Local, fusos como Brasil
		// podem jogar a data para o dia anterior e truncar 1 dia.
		baseUTC := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		expireDateUTC := baseUTC.AddDate(0, 0, int(expireDays))
		now := time.Now()
		todayLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		expireLocalDate := time.Date(expireDateUTC.Year(), expireDateUTC.Month(), expireDateUTC.Day(), 0, 0, 0, 0, now.Location())
		daysLeft := int(expireLocalDate.Sub(todayLocal).Hours() / 24)
		status := "active"
		if daysLeft < 0 {
			status = "expired"
		}
		return expireDateUTC.Format("02/01/2006"), daysLeft, status, nil
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

func isOfflineVPNState(state string) bool {
	state = strings.ToUpper(strings.TrimSpace(state))
	state = strings.ReplaceAll(state, "-", "_")
	state = strings.ReplaceAll(state, " ", "_")
	switch state {
	case "DISCONNECTED", "DESCONECTADO", "STOPPED", "PARADO", "STOPPING", "PARANDO", "DISCONNECTING", "DESCONECTANDO", "AUTH_FAILED", "NO_NETWORK", "ERROR", "ERRO", "FAILED", "FALHA":
		return true
	default:
		return false
	}
}

func detectCategory(s OnlineSession) string {
	if strings.TrimSpace(s.ConfigCategory) != "" {
		return strings.TrimSpace(s.ConfigCategory)
	}
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

func formatMillisOrUnix(v int64) string {
	if v <= 0 {
		return ""
	}
	// O JS do WebView costuma mandar Date.now() em milissegundos.
	if v > 1000000000000 {
		return time.UnixMilli(v).Format("02/01/2006 15:04:05")
	}
	return time.Unix(v, 0).Format("02/01/2006 15:04:05")
}

func writeJSONOrPixel(w http.ResponseWriter, r *http.Request, code int, v any) {
	if r.URL.Query().Get("pixel") == "1" || strings.HasPrefix(r.URL.Path, "/telemetry/pixel") || strings.HasPrefix(r.URL.Path, "/hb") {
		w.Header().Set("Content-Type", "image/gif")
		w.WriteHeader(code)
		_, _ = w.Write([]byte{71, 73, 70, 56, 57, 97, 1, 0, 1, 0, 128, 0, 0, 0, 0, 0, 255, 255, 255, 33, 249, 4, 1, 0, 0, 0, 0, 44, 0, 0, 0, 0, 1, 0, 1, 0, 0, 2, 2, 68, 1, 0, 59})
		return
	}
	writeJSON(w, code, v)
}

func shortTail(v string, n int) string {
	v = strings.TrimSpace(v)
	if len(v) <= n {
		return v
	}
	return v[len(v)-n:]
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
