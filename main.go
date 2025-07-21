package main
/*
*Note: This is stable version 
* 
*/
import (
	"context"
	"encoding/json"
	"fmt"
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
	Name string `json:"name"`
	Path string `json:"path"`
	Lang string `json:"lang"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type StreamStatus struct {
	Status      string     `json:"status"`
	VideoURL    string     `json:"videoUrl"`
	Magnet      string     `json:"magnet"`
	Downloading bool       `json:"downloading"`
	Progress    float64    `json:"progress"`
	FileSize    int64      `json:"fileSize"`
	FileType    string     `json:"fileType"`
	Subtitles   []Subtitle `json:"subtitles"`
}

var (
	client      *torrent.Client
	sessions    = make(map[string]*UserSession)
	sessionLock sync.Mutex
	appContext  context.Context
	appCancel   context.CancelFunc
)

func main() {
	// Initialize application context
	appContext, appCancel = context.WithCancel(context.Background())
	defer appCancel()

	logger.Info("=== Torrent Streamer API Starting ===")

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

	logger.Warn("=== Torrent Streamer API Shutting Down ===")
}

func runApplication() error {
	defer func() {
		if client != nil {
			client.Close()
			logger.Warn("Torrent client closed")
		}
	}()

	// Initialize torrent client
	if err := initializeTorrentClient(); err != nil {
		return fmt.Errorf("failed to initialize torrent client: %v", err)
	}

	// Set up routes
	setupRoutes()

	// Create necessary directories
	if err := createDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}

	// Start HTTP server
	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		defer recoverFromPanic("http-server")
		logger.Info("API Server starting on http://localhost:8080")
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

func createDirectories() error {
	dirs := []string{"logs", "subtitles", "static"}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Error("Error creating directory %s: %v", dir, err)
			return err
		}
	}

	logger.Info("All necessary directories created")
	return nil
}

func setupRoutes() {
	// API routes
	http.HandleFunc("/api/status", corsHandler(safeHTTPHandler("api-status", apiStatusHandler)))
	http.HandleFunc("/api/stream", corsHandler(safeHTTPHandler("api-stream", apiStreamHandler)))
	http.HandleFunc("/api/progress", corsHandler(safeHTTPHandler("api-progress", apiProgressHandler)))
	http.HandleFunc("/api/upload-subtitle", corsHandler(safeHTTPHandler("api-upload-subtitle", apiUploadSubtitleHandler)))

	// Add this line in setupRoutes() after the existing API routes
	http.HandleFunc("/api/reset-session", corsHandler(safeHTTPHandler("api-reset-session", apiResetSessionHandler)))

	// Media serving routes
	http.HandleFunc("/video", corsHandler(safeHTTPHandler("video", videoHandler)))
	http.HandleFunc("/subtitle", corsHandler(safeHTTPHandler("subtitle", subtitleHandler)))
	http.Handle("/subtitles/", http.StripPrefix("/subtitles/", http.FileServer(http.Dir("subtitles"))))

	// Static file serving
	http.Handle("/", http.FileServer(http.Dir("static/")))

	logger.Info("HTTP routes configured successfully")
}

func corsHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func apiStatusHandler(w http.ResponseWriter, r *http.Request) {
	session := getSession(w, r)
	
	status := StreamStatus{
		Status:    session.StatusMsg,
		Subtitles: session.Subtitles,
	}

	if session.Torrent != nil {
		meta := session.Torrent.Metainfo()
		magnet, _ := meta.MagnetV2()
		status.Magnet = magnet.String()
		status.Status = "Streaming: " + session.Torrent.Name()

		if session.File != nil {
			sessionID := getSessionID(w, r)
			status.VideoURL = "/video?session=" + sessionID
			completed := float64(session.File.BytesCompleted())
			total := float64(session.File.Length())
			if total > 0 {
				status.Progress = (completed / total) * 100
			}
			status.Downloading = status.Progress < 100
			status.FileSize = session.File.Length()

			fileName := session.File.Path()
			if dotIndex := strings.LastIndex(fileName, "."); dotIndex != -1 {
				status.FileType = fileName[dotIndex+1:]
			} else {
				status.FileType = "file"
			}
		}
	}

	respondJSON(w, APIResponse{Success: true, Data: status})
}

func apiStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		respondJSON(w, APIResponse{Success: false, Error: "Method not allowed"})
		return
	}

	var requestData struct {
		Magnet string `json:"magnet"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		respondJSON(w, APIResponse{Success: false, Error: "Invalid JSON"})
		return
	}

	if requestData.Magnet == "" {
		respondJSON(w, APIResponse{Success: false, Error: "Magnet link is required"})
		return
	}

	// Validate magnet link format
	if !strings.HasPrefix(requestData.Magnet, "magnet:?") {
		respondJSON(w, APIResponse{Success: false, Error: "Invalid magnet link format"})
		return
	}

	session := getSession(w, r)
	sessionID := getSessionID(w, r)

	// Truncate magnet link for logging
	magnetPreview := requestData.Magnet
	if len(magnetPreview) > 50 {
		magnetPreview = magnetPreview[:50] + "..."
	}
	logger.Info("Starting stream for magnet: %s (Session: %s)", magnetPreview, sessionID)

	go func() {
		defer recoverFromPanic("torrent-processing")
		processTorrent(session, requestData.Magnet, sessionID)
	}()

	respondJSON(w, APIResponse{Success: true, Message: "Stream started"})
}

func apiProgressHandler(w http.ResponseWriter, r *http.Request) {
	session := getSession(w, r)

	progress := 0.0
	status := "idle"

	if session.File != nil {
		completed := float64(session.File.BytesCompleted())
		total := float64(session.File.Length())
		if total > 0 {
			progress = (completed / total) * 100
		}

		if progress >= 100 {
			status = "completed"
		} else if progress > 0 {
			status = "downloading"
		}
	}

	data := map[string]interface{}{
		"progress": progress,
		"status":   status,
	}

	respondJSON(w, APIResponse{Success: true, Data: data})
}

func apiUploadSubtitleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		respondJSON(w, APIResponse{Success: false, Error: "Method not allowed"})
		return
	}

	sessionID := getSessionID(w, r)
	if sessionID == "" {
		respondJSON(w, APIResponse{Success: false, Error: "Invalid session"})
		return
	}

	file, header, err := r.FormFile("subtitle")
	if err != nil {
		logger.Error("Error reading subtitle file: %v", err)
		respondJSON(w, APIResponse{Success: false, Error: "Error reading subtitle file"})
		return
	}
	defer file.Close()

	// Validate file extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !isSubtitleFile(ext) {
		logger.Warn("Invalid subtitle file uploaded: %s", header.Filename)
		respondJSON(w, APIResponse{Success: false, Error: "Invalid subtitle file format"})
		return
	}

	// Validate file size (max 5MB)
	if header.Size > 5*1024*1024 {
		logger.Warn("Subtitle file too large: %s (%d bytes)", header.Filename, header.Size)
		respondJSON(w, APIResponse{Success: false, Error: "File too large (max 5MB)"})
		return
	}

	// Create safe filename
	safeFilename := strings.ReplaceAll(header.Filename, "..", "")
	safeFilename = strings.ReplaceAll(safeFilename, "/", "_")
	safeFilename = strings.ReplaceAll(safeFilename, "\\", "_")

	path := filepath.Join("subtitles", sessionID+"_"+safeFilename)

	dst, err := os.Create(path)
	if err != nil {
		logger.Error("Error creating subtitle file %s: %v", path, err)
		respondJSON(w, APIResponse{Success: false, Error: "Error saving file"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		logger.Error("Error writing subtitle file %s: %v", path, err)
		respondJSON(w, APIResponse{Success: false, Error: "Error saving file"})
		return
	}

	sessionLock.Lock()
	defer sessionLock.Unlock()

	if session, exists := sessions[sessionID]; exists {
		lang := detectSubtitleLanguage(header.Filename)
		session.Subtitles = append(session.Subtitles, Subtitle{
			Name: header.Filename,
			Path: "/subtitles/" + sessionID + "_" + safeFilename,
			Lang: lang,
		})
		logger.Info("Subtitle uploaded successfully: %s (Session: %s)", header.Filename, sessionID)
	}

	respondJSON(w, APIResponse{Success: true, Message: "Subtitle uploaded successfully"})
}

func apiResetSessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		respondJSON(w, APIResponse{Success: false, Error: "Method not allowed"})
		return
	}

	sessionID := getSessionID(w, r)
	
	sessionLock.Lock()
	defer sessionLock.Unlock()

	if session, exists := sessions[sessionID]; exists {
		// Clean up torrent if any
		if session.Torrent != nil {
			session.Torrent.Drop()
			logger.Info("Dropped torrent for session reset: %s", sessionID)
		}
		
		// Remove session
		delete(sessions, sessionID)
		logger.Info("Session reset: %s", sessionID)
	}

	// Clear the session cookie by setting it to expire immediately
	http.SetCookie(w, &http.Cookie{
		Name:     "ts_session_id",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1, // Expire immediately
	})

	respondJSON(w, APIResponse{Success: true, Message: "Session reset successfully"})
}

func processTorrent(session *UserSession, magnetLink, sessionID string) {
	sessionLock.Lock()
	defer sessionLock.Unlock()

	// Clean up existing torrent if any
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

	// Wait for torrent info with timeout
	select {
	case <-t.GotInfo():
		logger.Info("Got torrent info: %s", t.Name())
	case <-time.After(30 * time.Second):
		session.StatusMsg = "Timeout waiting for torrent metadata"
		logger.Error("Timeout waiting for torrent info")
		return
	}

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
				Path: "/subtitle?session=" + sessionID + "&file=" + f.Path(),
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
}

func videoHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		// Fallback to cookie-based session
		sessionID = getSessionID(w, r)
	}

	logger.Debug("Video request for session: %s", sessionID)

	sessionLock.Lock()
	session, exists := sessions[sessionID]
	sessionLock.Unlock()

	if !exists {
		logger.Warn("Video request for non-existent session: %s", sessionID)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	if session.File == nil {
		logger.Warn("Video request but no file available for session: %s", sessionID)
		http.Error(w, "No active file", http.StatusNotFound)
		return
	}

	// Update session activity
	session.LastActivity = time.Now()

	// Set appropriate headers for video streaming
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Create a new reader for this request
	reader := session.File.NewReader()
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Error("Error closing video reader: %v", err)
		}
	}()

	// Get file info
	fileLength := session.File.Length()
	fileName := session.File.Path()
	
	logger.Debug("Serving video stream for session: %s, file: %s, size: %d bytes", sessionID, fileName, fileLength)

	// Use ServeContent for proper range request handling
	http.ServeContent(w, r, "video.mp4", time.Now(), reader)
}

func subtitleHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	fileName := r.URL.Query().Get("file")

	if sessionID == "" || fileName == "" {
		logger.Warn("Invalid subtitle request - Session: %s, File: %s", sessionID, fileName)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	sessionLock.Lock()
	session, exists := sessions[sessionID]
	sessionLock.Unlock()

	if !exists || session.Torrent == nil {
		logger.Warn("Subtitle request for non-existent session: %s", sessionID)
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
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")

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

// Helper functions
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
		MaxAge:   60 * 60 * 24, // 1 day
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
	logger.Debug("Created new session: %s", sessionID)
	return session
}

func respondJSON(w http.ResponseWriter, response APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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
			for sessionID, session := range sessions {
				if now.Sub(session.LastActivity) > 30*time.Minute {
					if session.Torrent != nil {
						session.Torrent.Drop()
					}
					delete(sessions, sessionID)
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

// Utility functions
func isVideoFile(ext string) bool {
	videoExts := []string{".mp4", ".mkv", ".avi", ".mov", ".webm", ".flv", ".wmv", ".m4v", ".3gp"}
	for _, validExt := range videoExts {
		if ext == validExt {
			return true
		}
	}
	return false
}

func isSubtitleFile(ext string) bool {
	subtitleExts := []string{".srt", ".vtt", ".ass", ".ssa", ".sub", ".sbv"}
	for _, validExt := range subtitleExts {
		if ext == validExt {
			return true
		}
	}
	return false
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
				respondJSON(w, APIResponse{Success: false, Error: "Internal Server Error"})
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
	select {
	case <-appContext.Done():
		logger.Info("Application context cancelled")
	}
}
