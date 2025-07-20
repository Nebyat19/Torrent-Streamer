package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nebyat19/Torrent-Streamer/logger"
	"github.com/anacrolix/torrent"
	"github.com/asticode/go-astisub"
	"github.com/google/uuid"
)

type UserSession struct {
	Torrent      *torrent.Torrent
	File         *torrent.File
	Subtitles    []Subtitle
	LastActivity time.Time
	StatusMsg    string
}

type Subtitle struct {
	Name string
	Path string
	Lang string
}

var (
	client      *torrent.Client
	tmpl        *template.Template
	sessions    = make(map[string]*UserSession)
	sessionLock sync.Mutex
	appContext  context.Context
	appCancel   context.CancelFunc
)

func main() {
	// Initialize application context
	appContext, appCancel = context.WithCancel(context.Background())
	defer appCancel()

	logger.Info("=== Torrent Streamer Starting ===")

	// Start health monitoring
	go startHealthMonitor()

	// Run main application with recovery
	restartCount := 0
	maxRestarts := 5

	for restartCount <= maxRestarts {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("PANIC RECOVERED in main application: %v", r)
					restartCount++
					if restartCount <= maxRestarts {
						logger.Warn("Attempting restart %d/%d after 5s delay", restartCount, maxRestarts)
						time.Sleep(5 * time.Second)
					}
				}
			}()

			if err := runApplication(); err != nil {
				logger.Error("Application error: %v", err)
				restartCount++
				time.Sleep(5 * time.Second)
				return
			}

			// If we get here, the application exited normally
			restartCount = maxRestarts + 1 // Exit the loop
		}()
	}

	if restartCount > maxRestarts {
		logger.Error("Maximum restart attempts (%d) exceeded. Application will exit.", maxRestarts)
		os.Exit(1)
	}

	logger.Warn("=== Torrent Streamer Shutting Down ===")
}

func runApplication() error {
	defer func() {
		if client != nil {
			client.Close()
			logger.Warn("Torrent client closed")
		}
	}()

	// Initialize torrent client with recovery
	if err := initializeTorrentClient(); err != nil {
		return fmt.Errorf("failed to initialize torrent client: %v", err)
	}

	// Initialize template with recovery
	if err := initializeTemplate(); err != nil {
		return fmt.Errorf("failed to initialize template: %v", err)
	}

	// Set up routes with recovery wrappers
	setupRoutes()

	// Create subtitles directory
	if err := os.MkdirAll("subtitles", 0755); err != nil {
		logger.Error("Error creating subtitles directory: %v", err)
		return err
	}

	// Create logs directory
	if err := os.MkdirAll("logs", 0755); err != nil {
		logger.Error("Error creating logs directory: %v", err)
		return err
	}

	// Start HTTP server with recovery
	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		defer recoverFromPanic("http-server")
		logger.Info("Server starting on http://localhost:8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error: %v", err)
		}
	}()

	// Start session cleanup routine
	go func() {
		defer recoverFromPanic("session-cleanup")
		cleanupSessions()
	}()

	// Reset restart count after successful startup
	go func() {
		time.Sleep(30 * time.Second)
		logger.Info("Application startup successful")
	}()

	// Wait for shutdown signal
	waitForShutdown()

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error: %v", err)
	}

	return nil
}

func initializeTorrentClient() error {
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = os.TempDir()

	var err error
	client, err = torrent.NewClient(cfg)
	if err != nil {
		logger.Error("Failed to create torrent client: %v", err)
		return err
	}

	logger.Info("Torrent client initialized successfully")
	return nil
}

func initializeTemplate() error {
	// Create template with custom functions
	tmpl = template.New("index").Funcs(template.FuncMap{
		"formatFileSize": formatFileSize,
		"getFileIcon":    getFileIcon,
		"toUpper":        strings.ToUpper,
	})

	// Parse template
	var err error
	tmpl, err = tmpl.Parse(getHTMLTemplate())
	if err != nil {
		logger.Error("Error parsing template: %v", err)
		return err
	}

	logger.Info("Template initialized successfully")
	return nil
}

func setupRoutes() {
	http.HandleFunc("/", safeHTTPHandler("index", indexHandler))
	http.HandleFunc("/stream", safeHTTPHandler("stream", streamHandler))
	http.HandleFunc("/video", safeHTTPHandler("video", videoHandler))
	http.HandleFunc("/progress", safeHTTPHandler("progress", progressHandler))
	http.HandleFunc("/subtitle", safeHTTPHandler("subtitle", subtitleHandler))
	http.HandleFunc("/upload-subtitle", safeHTTPHandler("upload-subtitle", uploadSubtitleHandler))
	http.Handle("/subtitles/", http.StripPrefix("/subtitles/", http.FileServer(http.Dir("subtitles"))))

	logger.Info("HTTP routes configured")
}

func cleanupSessions() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sessionLock.Lock()
			now := time.Now()
			cleaned := 0
			for ip, session := range sessions {
				if now.Sub(session.LastActivity) > 30*time.Minute {
					if session.Torrent != nil {
						session.Torrent.Drop()
					}
					delete(sessions, ip)
					cleaned++
				}
			}
			sessionLock.Unlock()

			if cleaned > 0 {
				logger.Info("Cleaned up %d inactive sessions", cleaned)
			}
		case <-appContext.Done():
			logger.Info("Session cleanup stopping...")
			return
		}
	}
}

func getClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func getSessionID(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("ts_session_id")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	// Generate new session ID
	sessionID := uuid.New().String()
	http.SetCookie(w, &http.Cookie{
		Name:     "ts_session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		// Secure: true, // Uncomment if using HTTPS
		MaxAge: 60 * 60 * 24, // 1 day
	})
	return sessionID
}

func getSession(w http.ResponseWriter, r *http.Request) *UserSession {
	sessionID := getSessionID(w, r)
	sessionLock.Lock()
	defer sessionLock.Unlock()
	if session, exists := sessions[sessionID]; exists {
		session.LastActivity = time.Now()
		return session
	}
	session := &UserSession{
		LastActivity: time.Now(),
		StatusMsg:    "Ready to stream",
	}
	sessions[sessionID] = session
	logger.Debug("Created new session for sessionID: %s", sessionID)
	return session
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	session := getSession(w, r)
	data := struct {
		Status      string
		VideoURL    string
		Magnet      string
		Downloading bool
		Progress    float64
		IP          string
		FileSize    int64
		FileType    string
		Subtitles   []Subtitle
	}{
		Status:    session.StatusMsg,
		Magnet:    "",
		Progress:  0,
		IP:        getClientIP(r),
		Subtitles: session.Subtitles,
	}

	if session.Torrent != nil {
		meta := session.Torrent.Metainfo()
		magnet, _ := meta.MagnetV2()
		data.Magnet = magnet.String()
		data.Status = "Streaming: " + session.Torrent.Name()

		if session.File != nil {
			data.VideoURL = "/video?ip=" + getClientIP(r)
			completed := float64(session.File.BytesCompleted())
			total := float64(session.File.Length())
			data.Progress = (completed / total) * 100
			data.Downloading = data.Progress < 100
			data.FileSize = session.File.Length()

			fileName := session.File.Path()
			if dotIndex := strings.LastIndex(fileName, "."); dotIndex != -1 {
				data.FileType = fileName[dotIndex+1:]
			} else {
				data.FileType = "file"
			}
		}
	}

	if err := tmpl.Execute(w, data); err != nil {
		logger.Error("Template execution error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	magnetLink := r.FormValue("magnet")
	if magnetLink == "" {
		logger.Warn("Empty magnet link received from IP: %s", getClientIP(r))
		http.Error(w, "Magnet link is required", http.StatusBadRequest)
		return
	}

	session := getSession(w, r)
	clientIP := getClientIP(r)

	// Truncate magnet link for logging
	magnetPreview := magnetLink
	if len(magnetPreview) > 50 {
		magnetPreview = magnetPreview[:50] + "..."
	}
	logger.Info("Starting stream for magnet: %s (IP: %s)", magnetPreview, clientIP)

	go func() {
		defer recoverFromPanic("torrent-processing")

		sessionLock.Lock()
		defer sessionLock.Unlock()

		if session.Torrent != nil {
			session.Torrent.Drop()
			session.Torrent = nil
			session.File = nil
			session.Subtitles = nil
		}

		session.StatusMsg = "Connecting to peers..."

		t, err := client.AddMagnet(magnetLink)
		if err != nil {
			session.StatusMsg = "Error: " + err.Error()
			logger.Error("Error adding magnet: %v", err)
			return
		}

		session.Torrent = t
		session.StatusMsg = "Fetching torrent metadata..."
		logger.Info("Torrent added, waiting for info...")

		<-t.GotInfo()
		logger.Info("Got torrent info: %s", t.Name())

		session.StatusMsg = "Finding video file and subtitles..."
		videoFound := false
		subtitleCount := 0

		for _, f := range t.Files() {
			ext := strings.ToLower(filepath.Ext(f.Path()))

			if session.File == nil && isVideoFile(ext) {
				session.File = f
				f.Download()
				logger.Info("Found video file: %s (%.2f MB)", f.Path(), float64(f.Length())/1024/1024)
				videoFound = true
				continue
			}

			if isSubtitleFile(ext) {
				f.Download()
				lang := detectSubtitleLanguage(f.Path())
				session.Subtitles = append(session.Subtitles, Subtitle{
					Name: filepath.Base(f.Path()),
					Path: "/subtitle?ip=" + clientIP + "&file=" + f.Path(),
					Lang: lang,
				})
				logger.Debug("Found subtitle: %s", f.Path())
				subtitleCount++
			}
		}

		if videoFound {
			session.StatusMsg = "Ready to play: " + session.Torrent.Name()
			logger.Info("Stream ready for: %s (%d subtitles found)", session.Torrent.Name(), subtitleCount)
		} else {
			session.StatusMsg = "No video file found in torrent"
			logger.Warn("No video file found in torrent: %s", session.Torrent.Name())
		}
	}()

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func videoHandler(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		ip = getClientIP(r)
	}

	sessionLock.Lock()
	session, exists := sessions[getSessionID(w, r)]
	sessionLock.Unlock()

	if !exists || session.File == nil {
		logger.Warn("Video request for non-existent session: %s", getSessionID(w, r))
		http.Error(w, "No active file", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-cache")

	reader := session.File.NewReader()
	defer reader.Close()

	logger.Debug("Serving video stream for IP: %s", ip)
	http.ServeContent(w, r, "video.mp4", time.Now(), reader)
}

func progressHandler(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		ip = getClientIP(r)
	}

	sessionLock.Lock()
	session, exists := sessions[getSessionID(w, r)]
	sessionLock.Unlock()

	progress := 0.0
	if exists && session.File != nil {
		completed := float64(session.File.BytesCompleted())
		total := float64(session.File.Length())
		if total > 0 {
			progress = (completed / total) * 100
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"progress": %.1f}`, progress)))
}

func subtitleHandler(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	fileName := r.URL.Query().Get("file")

	if ip == "" || fileName == "" {
		logger.Warn("Invalid subtitle request - IP: %s, File: %s", ip, fileName)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	sessionLock.Lock()
	session, exists := sessions[getSessionID(w, r)]
	sessionLock.Unlock()

	if !exists || session.Torrent == nil {
		logger.Warn("Subtitle request for non-existent session: %s", getSessionID(w, r))
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	var subFile *torrent.File
	for _, f := range session.Torrent.Files() {
		if f.Path() == fileName {
			subFile = f
			break
		}
	}

	if subFile == nil {
		logger.Warn("Subtitle file not found: %s", fileName)
		http.Error(w, "Subtitle not found", http.StatusNotFound)
		return
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	w.Header().Set("Content-Type", "text/vtt")

	if ext != ".vtt" {
		reader := subFile.NewReader()
		defer reader.Close()

		var subs *astisub.Subtitles
		var err error

		switch ext {
		case ".srt":
			subs, err = astisub.ReadFromSRT(reader)
		case ".ass", ".ssa":
			subs, err = astisub.ReadFromSSA(reader)
		default:
			logger.Error("Unsupported subtitle format: %s", ext)
			http.Error(w, "Unsupported subtitle format", http.StatusBadRequest)
			return
		}

		if err != nil {
			logger.Error("Error parsing subtitle %s: %v", fileName, err)
			http.Error(w, "Error parsing subtitle", http.StatusInternalServerError)
			return
		}

		err = subs.WriteToWebVTT(w)
		if err != nil {
			logger.Error("Error converting subtitle %s: %v", fileName, err)
			http.Error(w, "Error converting subtitle", http.StatusInternalServerError)
			return
		}

		logger.Debug("Served converted subtitle: %s", fileName)
		return
	}

	// Serve VTT file directly
	reader := subFile.NewReader()
	defer reader.Close()
	io.Copy(w, reader)
	logger.Debug("Served VTT subtitle: %s", fileName)
}

func uploadSubtitleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := r.FormValue("ip")
	if ip == "" {
		logger.Warn("Upload subtitle request without IP")
		http.Error(w, "Invalid session", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("subtitle")
	if err != nil {
		logger.Error("Error reading subtitle file: %v", err)
		http.Error(w, "Error reading subtitle file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !isSubtitleFile(ext) {
		logger.Warn("Invalid subtitle file uploaded: %s", header.Filename)
		http.Error(w, "Invalid subtitle file format", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll("subtitles", 0755); err != nil {
		logger.Error("Error creating subtitles directory: %v", err)
		http.Error(w, "Error creating directory", http.StatusInternalServerError)
		return
	}

	// Create safe filename
	safeFilename := strings.ReplaceAll(header.Filename, "..", "")
	path := filepath.Join("subtitles", ip+"_"+safeFilename)

	dst, err := os.Create(path)
	if err != nil {
		logger.Error("Error creating subtitle file %s: %v", path, err)
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		logger.Error("Error writing subtitle file %s: %v", path, err)
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	sessionLock.Lock()
	defer sessionLock.Unlock()

	if session, exists := sessions[getSessionID(w, r)]; exists {
		lang := detectSubtitleLanguage(header.Filename)
		session.Subtitles = append(session.Subtitles, Subtitle{
			Name: header.Filename,
			Path: "/subtitles/" + ip + "_" + safeFilename,
			Lang: lang,
		})
		logger.Info("Subtitle uploaded successfully: %s (IP: %s)", header.Filename, ip)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true}`))
}

// Utility functions
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func getFileIcon(fileType string) string {
	switch strings.ToLower(fileType) {
	case "mp4", "mkv", "avi", "mov", "webm":
		return "fas fa-file-video"
	case "mp3", "wav", "flac", "aac":
		return "fas fa-file-audio"
	case "jpg", "jpeg", "png", "gif", "webp":
		return "fas fa-file-image"
	case "pdf":
		return "fas fa-file-pdf"
	case "zip", "rar", "7z", "tar", "gz":
		return "fas fa-file-archive"
	default:
		return "fas fa-file"
	}
}

func isVideoFile(ext string) bool {
	switch ext {
	case ".mp4", ".mkv", ".avi", ".mov", ".webm", ".flv", ".wmv":
		return true
	default:
		return false
	}
}

func isSubtitleFile(ext string) bool {
	switch ext {
	case ".srt", ".vtt", ".ass", ".ssa", ".sub":
		return true
	default:
		return false
	}
}

func detectSubtitleLanguage(filename string) string {
	filename = strings.ToLower(filename)

	langs := map[string]string{
		"english": "en", ".en.": "en", "eng.": "en", ".eng.": "en",
		"french": "fr", ".fr.": "fr", "fra.": "fr", ".fra.": "fr",
		"spanish": "es", ".es.": "es", "spa.": "es", ".spa.": "es",
		"german": "de", ".de.": "de", "ger.": "de", ".ger.": "de",
		"japanese": "ja", ".ja.": "ja", "jpn.": "ja", ".jpn.": "ja",
		"chinese": "zh", ".zh.": "zh", "chi.": "zh", ".chi.": "zh",
		"korean": "ko", ".ko.": "ko", "kor.": "ko", ".kor.": "ko",
		"russian": "ru", ".ru.": "ru", "rus.": "ru", ".rus.": "ru",
		"italian": "it", ".it.": "it", "ita.": "it", ".ita.": "it",
		"portuguese": "pt", ".pt.": "pt", "por.": "pt", ".por.": "pt",
		"dutch": "nl", ".nl.": "nl", "nld.": "nl", ".nld.": "nl",
	}

	for pattern, code := range langs {
		if strings.Contains(filename, pattern) {
			return code
		}
	}
	return "und" // undefined
}

// Recovery functions
func recoverFromPanic(operation string) {
	if r := recover(); r != nil {
		logger.Warn("PANIC RECOVERED in %s: %v", operation, r)
	}
}

func safeHTTPHandler(name string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("PANIC in HTTP handler %s: %v", name, rec)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		handler(w, r)
	}
}

func startHealthMonitor() {
	defer recoverFromPanic("health-monitor")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Simple health check
			sessionLock.Lock()
			sessionCount := len(sessions)
			sessionLock.Unlock()

			logger.Debug("Health check - Active sessions: %d", sessionCount)

		case <-appContext.Done():
			logger.Info("Health monitor stopping...")
			return
		}
	}
}

func waitForShutdown() {
	// Simple shutdown signal handling
	// In production, you'd want proper signal handling
	select {
	case <-appContext.Done():
		logger.Info("Application context cancelled")
	}
}
