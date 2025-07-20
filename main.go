package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/asticode/go-astisub"
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
)

func main() {
	// Initialize torrent client
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = os.TempDir()
	var err error
	client, err = torrent.NewClient(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create template with custom functions
	tmpl = template.New("index").Funcs(template.FuncMap{
		"formatFileSize": formatFileSize,
		"getFileIcon":    getFileIcon,
		"toUpper":        strings.ToUpper,
	})

	// Parse template with refined typography
	tmpl, err = tmpl.Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Torrent Streamer</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&family=Source+Sans+Pro:wght@300;400;500;600&display=swap" rel="stylesheet">
    <style>
        :root {
            --primary: rgb(227, 151, 19);
            --primary-hover: rgb(254, 174, 0);
            --secondary:rgb(56, 58, 60);
            --text:rgb(233, 234, 237);
            --text-light:rgb(202, 204, 206);
            --text-muted:rgb(220, 226, 235);
            --border: #e2e8f0;
            --border-light: #f1f5f9;
            --success: #10b981;
            --error: #ef4444;
            --warning: #f59e0b;
            --background: #ffffff;
            --surface: #f8fafc;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'Outfit', 'Source Sans Pro', -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', system-ui, sans-serif;
            font-size: 13px;
            line-height: 1.4;
            color: var(--text);
            background: var(--surface);
            min-height: 100vh;
            padding: 0.75rem;
            font-weight: 400;
            letter-spacing: -0.01em;
			background:grey;
        }

        .container {
            max-width: 950px;
            margin: 0 auto;
            background: var(--background);
            border-radius: 10px;
            border: 1px solid var(--border-light);
            overflow: hidden;
			background: black;
			
            box-shadow: 0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06);
        }

        .header {
            padding: 1.5rem 1.5rem 1.25rem;
            text-align: center;
            border-bottom: 1px solid var(--border-light);
        }

        .header h1 {
            font-size: 1.25rem;
            font-weight: 600;
            color: var(--text);
            margin-bottom: 0.375rem;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 0.5rem;
            letter-spacing: -0.02em;
        }

        .header p {
            font-size: 0.75rem;
            color: var(--text-muted);
            font-weight: 400;
            letter-spacing: -0.005em;
        }

        .content {
            padding: 1.5rem;
        }

        .form-group {
            margin-bottom: 1.25rem;
        }

        .form-label {
            display: block;
            font-size: 0.75rem;
            font-weight: 500;
            color: var(--text);
            margin-bottom: 0.5rem;
            letter-spacing: -0.005em;
        }

        .form-input {
            width: 100%;
            padding: 0.625rem 0.875rem;
            border: 1px solid var(--border);
            border-radius: 7px;
            font-size: 0.75rem;
            font-family: inherit;
            transition: all 0.2s ease;
            background: var(--background);
            letter-spacing: -0.005em;
        }

        .form-input:focus {
            outline: none;
            border-color: var(--primary);
            box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
        }

        .form-input::placeholder {
            color: var(--text-muted);
            font-size: 0.7rem;
        }

        .btn {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            gap: 0.375rem;
            padding: 0.625rem 1.25rem;
            font-size: 0.75rem;
            font-weight: 500;
            border: none;
            border-radius: 7px;
            cursor: pointer;
            transition: all 0.2s ease;
            text-decoration: none;
            font-family: inherit;
            letter-spacing: -0.005em;
        }

        .btn-primary {
            background: var(--primary);
            color: white;
            width: 100%;
        }

        .btn-primary:hover:not(:disabled) {
            background: var(--primary-hover);
        }

        .btn-primary:disabled {
            background: var(--text-muted);
            cursor: not-allowed;
        }

        .btn-secondary {
            background: var(--secondary);
            color: var(--text);
            border: 1px solid var(--border);
        }

        .btn-secondary:hover {
            background: var(--border-light);
        }

        .btn-secondary.active {
            background: var(--primary);
            color: white;
            border-color: var(--primary);
        }

        .status-card {
            background: var(--secondary);
            border: 1px solid var(--border-light);
            border-radius: 7px;
            padding: 0.875rem;
            margin: 1.25rem 0;
            display: flex;
            align-items: center;
            gap: 0.625rem;
            font-size: 0.7rem;
            letter-spacing: -0.005em;
        }

        .status-icon {
            color: var(--primary);
            font-size: 0.875rem;
        }

        .progress-container {
            background: var(--border-light);
            border-radius: 5px;
            overflow: hidden;
            margin: 1.25rem 0;
            height: 6px;
        }

        .progress-bar {
            height: 100%;
            background: var(--primary);
            transition: width 0.3s ease;
            border-radius: 5px;
        }

        .progress-text {
            font-size: 0.65rem;
            color: var(--text-muted);
            text-align: center;
            margin-top: 0.5rem;
            letter-spacing: -0.005em;
        }

        .file-info {
            display: flex;
            gap: 1.25rem;
            margin: 1.25rem 0;
            padding: 0.875rem;
            background: var(--secondary);
            border-radius: 7px;
            border: 1px solid var(--border-light);
        }

        .file-info-item {
            display: flex;
            align-items: center;
            gap: 0.375rem;
            font-size: 0.65rem;
            color: var(--text-light);
            letter-spacing: -0.005em;
        }

        .file-info-icon {
            color: var(--primary);
            font-size: 0.75rem;
        }

        .video-container {
            margin: 1.25rem 0;
            border-radius: 7px;
            overflow: hidden;
            background: #000;
            border: 1px solid var(--border-light);
        }

        .video-player {
            width: 100%;
            display: block;
            max-height: 380px;
        }

        .subtitle-controls {
            display: flex;
            gap: 0.375rem;
            margin: 0.875rem 0;
            flex-wrap: wrap;
        }

        .subtitle-upload {
            margin-top: 1.25rem;
            padding-top: 1.25rem;
            border-top: 1px solid var(--border-light);
        }

        .file-input-wrapper {
            position: relative;
            display: inline-block;
            margin: 0.375rem 0;
        }

        .file-input {
            position: absolute;
            opacity: 0;
            width: 100%;
            height: 100%;
            cursor: pointer;
        }

        .file-input-label {
            display: inline-flex;
            align-items: center;
            gap: 0.375rem;
            padding: 0.5rem 0.875rem;
            background: var(--secondary);
            border: 1px solid var(--border);
            border-radius: 6px;
            font-size: 0.65rem;
            color: var(--text-light);
            cursor: pointer;
            transition: all 0.2s ease;
            letter-spacing: -0.005em;
        }

        .file-input-label:hover {
            background: var(--border-light);
        }

        .notification {
            position: fixed;
            bottom: 1rem;
            right: 1rem;
            max-width: 300px;
            padding: 0.875rem;
            background: var(--background);
            border: 1px solid var(--border);
            border-radius: 7px;
            box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
            display: flex;
            align-items: center;
            gap: 0.625rem;
            font-size: 0.75rem;
            z-index: 1000;
            transform: translateY(100px);
            opacity: 0;
            transition: all 0.3s ease;
            letter-spacing: -0.005em;
        }

        .notification.show {
            transform: translateY(0);
            opacity: 1;
        }

        .notification.success {
            border-color: var(--success);
        }

        .notification.error {
            border-color: var(--error);
        }

        .notification-icon {
            font-size: 0.875rem;
        }

        .notification.success .notification-icon {
            color: var(--success);
        }

        .notification.error .notification-icon {
            color: var(--error);
        }

        .loader {
            width: 14px;
            height: 14px;
            border: 2px solid var(--border-light);
            border-top: 2px solid var(--primary);
            border-radius: 50%;
            animation: spin 1s linear infinite;
        }

        .icon {
            font-size: 0.875rem;
        }

        .icon-sm {
            font-size: 0.75rem;
        }

        .icon-xs {
            font-size: 0.65rem;
        }

        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }

        @media (max-width: 640px) {
            body {
                padding: 0.5rem;
                font-size: 12px;
            }
            
            .container {
                border-radius: 7px;
            }
            
            .header {
                padding: 1.25rem 1rem 1rem;
            }

            .header h1 {
                font-size: 1.125rem;
            }

            .header p {
                font-size: 0.7rem;
            }
            
            .content {
                padding: 1.25rem 1rem;
            }

            .form-input {
                font-size: 0.7rem;
            }

            .btn {
                font-size: 0.7rem;
                padding: 0.5rem 1rem;
            }
            
            .subtitle-controls {
                gap: 0.25rem;
            }
            
            .btn-secondary {
                font-size: 0.65rem;
                padding: 0.375rem 0.625rem;
            }

            .file-info {
                flex-direction: column;
                gap: 0.625rem;
            }

            .status-card {
                font-size: 0.65rem;
            }
        }

        /* Enhanced readability for small text */
        @media (max-width: 480px) {
            body {
                font-size: 11px;
            }

            .header h1 {
                font-size: 1rem;
            }

            .header p {
                font-size: 0.65rem;
            }

            .form-input {
                font-size: 0.65rem;
                padding: 0.5rem 0.75rem;
            }

            .btn {
                font-size: 0.65rem;
                padding: 0.5rem 0.875rem;
            }

            .status-card {
                font-size: 0.6rem;
                padding: 0.75rem;
            }

            .file-info-item {
                font-size: 0.6rem;
            }

            .progress-text {
                font-size: 0.6rem;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polygon points="23 7 16 12 23 17 23 7"></polygon>
                    <rect x="1" y="5" width="15" height="14" rx="2" ry="2"></rect>
                </svg>
                Torrent Streamer
            </h1>
            <p>Stream video content directly from magnet links</p>
        </div>
        
        <div class="content">
            <form id="streamForm" action="/stream" method="post">
                <div class="form-group">
                    <label for="magnet" class="form-label">Magnet Link</label>
                    <input type="text" id="magnet" name="magnet" class="form-input" required
                           placeholder="magnet:?xt=urn:btih:..." value="{{.Magnet}}">
                </div>
                <button type="submit" id="streamBtn" class="btn btn-primary">
                    <span id="btnText">
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polygon points="5 3 19 12 5 21 5 3"></polygon>
                        </svg>
                        Start Streaming
                    </span>
                    <div id="btnLoader" class="loader" style="display: none;"></div>
                </button>
            </form>

            <div class="status-card">
                <svg class="status-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="12" cy="12" r="10"></circle>
                    <line x1="12" y1="16" x2="12" y2="12"></line>
                    <line x1="12" y1="8" x2="12.01" y2="8"></line>
                </svg>
                <span id="statusText">{{.Status}}</span>
            </div>

            {{if .Downloading}}
            <div class="progress-container">
                <div id="progressBar" class="progress-bar" style="width: {{.Progress}}%"></div>
				</div>
				<div class="progress-text">{{printf "%.1f" .Progress}}% downloaded</div>
            {{end}}

            {{if .VideoURL}}
            <div class="file-info">
                <div class="file-info-item">
                    <svg class="file-info-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polygon points="23 7 16 12 23 17 23 7"></polygon>
                        <rect x="1" y="5" width="15" height="14" rx="2" ry="2"></rect>
                    </svg>
                    <span>{{.FileType | toUpper}}</span>
                </div>
                <div class="file-info-item">
                    <svg class="file-info-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                        <polyline points="14,2 14,8 20,8"></polyline>
                    </svg>
                    <span>{{.FileSize | formatFileSize}}</span>
                </div>
            </div>

            <div class="video-container">
                <video controls autoplay id="videoPlayer" class="video-player" crossorigin="anonymous">
                    <source src="{{.VideoURL}}" type="video/mp4">
                    <track kind="subtitles" id="subtitleTrack" label="None" srclang="none" default>
                    Your browser does not support HTML5 video.
                </video>
            </div>

            {{if .Subtitles}}
            <div class="subtitle-controls">
                <button class="btn btn-secondary active" onclick="setSubtitle('none')">
                    <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="m9 9 3 3m0 0 3-3m-3 3V4m0 5H4m16 0h-4"></path>
                    </svg>
                    None
                </button>
                {{range .Subtitles}}
                <button class="btn btn-secondary" onclick="setSubtitle('{{.Path}}', '{{.Lang}}')">
                    <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z"></path>
                    </svg>
                    {{.Name}}
                </button>
                {{end}}
            </div>
            {{end}}

            <div class="subtitle-upload">
                <label class="form-label">Upload Custom Subtitles</label>
                <div class="file-input-wrapper">
                    <input type="file" id="subtitleFile" class="file-input" accept=".srt,.vtt,.ass">
                    <label for="subtitleFile" class="file-input-label">
                        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                            <polyline points="7,10 12,15 17,10"></polyline>
                            <line x1="12" y1="15" x2="12" y2="3"></line>
                        </svg>
                        Choose File
                    </label>
                </div>
                <button onclick="uploadSubtitle()" class="btn btn-secondary" style="margin-left: 0.375rem;">
                    <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                        <polyline points="17,8 12,3 7,8"></polyline>
                        <line x1="12" y1="3" x2="12" y2="15"></line>
                    </svg>
                    Upload
                </button>
            </div>
            {{end}}
        </div>
    </div>

    <div id="notification" class="notification">
        <svg class="notification-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="20,6 9,17 4,12"></polyline>
        </svg>
        <span id="notificationText"></span>
    </div>

    <script>
        document.getElementById('streamForm').addEventListener('submit', function(e) {
            e.preventDefault();
            const btn = document.getElementById('streamBtn');
            const btnText = document.getElementById('btnText');
            const loader = document.getElementById('btnLoader');
            const statusText = document.getElementById('statusText');

            btn.disabled = true;
            btnText.textContent = 'Processing...';
            loader.style.display = 'block';
            statusText.textContent = 'Connecting to torrent network...';

            fetch('/stream', {
                method: 'POST',
                body: new URLSearchParams(new FormData(this)),
                headers: {
                    'Content-Type': 'application/x-www-form-urlencoded',
                }
            })
            .then(response => {
                if (response.redirected) {
                    window.location.href = response.url;
                }
                return response.text();
            })
            .catch(error => {
                showNotification('Error: ' + error.message, 'error');
            })
            .finally(() => {
                btn.disabled = false;
                btnText.innerHTML = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg> Start Streaming';
                loader.style.display = 'none';
            });
        });

        function showNotification(message, type) {
            const notification = document.getElementById('notification');
            const notificationText = document.getElementById('notificationText');
            
            notificationText.textContent = message;
            notification.className = 'notification ' + type;
            notification.classList.add('show');

            setTimeout(() => {
                notification.classList.remove('show');
            }, 4000);
        }

        function setSubtitle(url, lang) {
            const video = document.getElementById('videoPlayer');
            const track = document.getElementById('subtitleTrack');
            
            if (url === 'none') {
                track.src = '';
                track.label = 'None';
                track.srclang = 'none';
                document.querySelectorAll('.subtitle-controls .btn').forEach(btn => {
                    btn.classList.remove('active');
                });
                document.querySelector('.subtitle-controls .btn').classList.add('active');
                return;
            }
            
            track.src = url;
            track.label = lang || 'Custom';
            track.srclang = lang || 'custom';
            track.mode = 'showing';
            
            document.querySelectorAll('.subtitle-controls .btn').forEach(btn => {
                btn.classList.remove('active');
            });
            event.target.classList.add('active');
        }

        function uploadSubtitle() {
            const fileInput = document.getElementById('subtitleFile');
            if (!fileInput.files.length) {
                showNotification('Please select a subtitle file first', 'error');
                return;
            }
            
            const formData = new FormData();
            formData.append('subtitle', fileInput.files[0]);
            formData.append('ip', '{{.IP}}');
            
            fetch('/upload-subtitle', {
                method: 'POST',
                body: formData
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    showNotification('Subtitle uploaded successfully', 'success');
                    setTimeout(() => location.reload(), 1000);
                } else {
                    showNotification('Error: ' + data.error, 'error');
                }
            })
            .catch(error => {
                showNotification('Error uploading subtitle', 'error');
            });
        }

        {{if .Downloading}}
        function updateProgress() {
            fetch('/progress?ip={{.IP}}')
                .then(response => response.json())
                .then(data => {
                    const progressBar = document.getElementById('progressBar');
                    const progressText = document.querySelector('.progress-text');
                    if (progressBar) {
                        progressBar.style.width = data.progress + '%';
                    }
                    if (progressText) {
                        progressText.textContent = data.progress.toFixed(1) + '% downloaded';
                    }
                    if (data.progress < 100) {
                        setTimeout(updateProgress, 1000);
                    }
                });
        }
        setTimeout(updateProgress, 1000);
        {{end}}
    </script>
</body>
</html>
    `)
	if err != nil {
		log.Fatalf("Error parsing template: %v", err)
	}

	// Set up routes (same as before)
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/stream", streamHandler)
	http.HandleFunc("/video", videoHandler)
	http.HandleFunc("/progress", progressHandler)
	http.HandleFunc("/subtitle", subtitleHandler)
	http.HandleFunc("/upload-subtitle", uploadSubtitleHandler)
	http.Handle("/subtitles/", http.StripPrefix("/subtitles/", http.FileServer(http.Dir("subtitles"))))

	if err := os.MkdirAll("subtitles", 0755); err != nil {
		log.Fatalf("Error creating subtitles directory: %v", err)
	}

	log.Println("Server started on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// ... (rest of the functions remain the same as in the previous version)
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

func getSession(r *http.Request) *UserSession {
	ip := getClientIP(r)
	sessionLock.Lock()
	defer sessionLock.Unlock()

	if session, exists := sessions[ip]; exists {
		session.LastActivity = time.Now()
		return session
	}

	session := &UserSession{
		LastActivity: time.Now(),
		StatusMsg:    "Ready to stream",
	}
	sessions[ip] = session
	return session
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	session := getSession(r)
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

	tmpl.Execute(w, data)
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	magnetLink := r.FormValue("magnet")
	if magnetLink == "" {
		http.Error(w, "Magnet link is required", http.StatusBadRequest)
		return
	}

	session := getSession(r)

	go func() {
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
			log.Printf("Error adding magnet: %v", err)
			return
		}

		session.Torrent = t
		session.StatusMsg = "Fetching torrent metadata..."

		<-t.GotInfo()

		session.StatusMsg = "Finding video file and subtitles..."
		for _, f := range t.Files() {
			ext := strings.ToLower(filepath.Ext(f.Path()))

			if session.File == nil && isVideoFile(ext) {
				session.File = f
				f.Download()
				continue
			}

			if isSubtitleFile(ext) {
				f.Download()
				lang := detectSubtitleLanguage(f.Path())
				session.Subtitles = append(session.Subtitles, Subtitle{
					Name: filepath.Base(f.Path()),
					Path: "/subtitle?ip=" + getClientIP(r) + "&file=" + f.Path(),
					Lang: lang,
				})
			}
		}

		if session.File != nil {
			session.StatusMsg = "Ready to play: " + session.Torrent.Name()
		} else {
			session.StatusMsg = "No video file found in torrent"
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
	session, exists := sessions[ip]
	sessionLock.Unlock()

	if !exists || session.File == nil {
		http.Error(w, "No active file", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	reader := session.File.NewReader()
	defer reader.Close()
	http.ServeContent(w, r, "video.mp4", time.Now(), reader)
}

func subtitleHandler(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	fileName := r.URL.Query().Get("file")

	if ip == "" || fileName == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	sessionLock.Lock()
	session, exists := sessions[ip]
	sessionLock.Unlock()

	if !exists || session.Torrent == nil {
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
		http.Error(w, "Subtitle not found", http.StatusNotFound)
		return
	}

	ext := strings.ToLower(filepath.Ext(fileName))
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
			http.Error(w, "Unsupported subtitle format", http.StatusBadRequest)
			return
		}

		if err != nil {
			http.Error(w, "Error parsing subtitle", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/vtt")
		err = subs.WriteToWebVTT(w)
		if err != nil {
			http.Error(w, "Error converting subtitle", http.StatusInternalServerError)
			return
		}
		return
	}

	w.Header().Set("Content-Type", "text/vtt")
	reader := subFile.NewReader()
	defer reader.Close()
	io.Copy(w, reader)
}

func uploadSubtitleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := r.FormValue("ip")
	if ip == "" {
		http.Error(w, "Invalid session", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("subtitle")
	if err != nil {
		http.Error(w, "Error reading subtitle file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if err := os.MkdirAll("subtitles", 0755); err != nil {
		http.Error(w, "Error creating directory", http.StatusInternalServerError)
		return
	}

	path := filepath.Join("subtitles", ip+"_"+header.Filename)
	dst, err := os.Create(path)
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	sessionLock.Lock()
	defer sessionLock.Unlock()

	if session, exists := sessions[ip]; exists {
		lang := detectSubtitleLanguage(header.Filename)
		session.Subtitles = append(session.Subtitles, Subtitle{
			Name: header.Filename,
			Path: "/subtitles/" + ip + "_" + header.Filename,
			Lang: lang,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true}`))
}

func progressHandler(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		ip = getClientIP(r)
	}

	sessionLock.Lock()
	session, exists := sessions[ip]
	sessionLock.Unlock()

	progress := 0.0
	if exists && session.File != nil {
		completed := float64(session.File.BytesCompleted())
		total := float64(session.File.Length())
		progress = (completed / total) * 100
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"progress": ` + fmt.Sprintf("%.1f", progress) + `}`))
}

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
	case ".mp4", ".mkv", ".avi", ".mov", ".webm":
		return true
	default:
		return false
	}
}

func isSubtitleFile(ext string) bool {
	switch ext {
	case ".srt", ".vtt", ".ass", ".ssa":
		return true
	default:
		return false
	}
}

func detectSubtitleLanguage(filename string) string {
	filename = strings.ToLower(filename)

	langs := map[string]string{
		"english": "en", ".en.": "en", "eng.": "en",
		"french": "fr", ".fr.": "fr", "fra.": "fr",
		"spanish": "es", ".es.": "es", "spa.": "es",
		"german": "de", ".de.": "de", "ger.": "de",
		"japanese": "ja", ".ja.": "ja", "jpn.": "ja",
		"chinese": "zh", ".zh.": "zh", "chi.": "zh",
		"korean": "ko", ".ko.": "ko", "kor.": "ko",
		"russian": "ru", ".ru.": "ru", "rus.": "ru",
	}

	for pattern, code := range langs {
		if strings.Contains(filename, pattern) {
			return code
		}
	}

	return "und"
}