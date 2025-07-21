class TorrentStreamer {
    constructor() {
      this.apiBase = "/api"
      this.currentSubtitles = []
      this.progressInterval = null
      this.statusInterval = null
      this.currentVideoUrl = null // Add this line
  
      this.initializeEventListeners()
      this.startStatusPolling()
    }
  
    initializeEventListeners() {
      // Form submission
      document.getElementById("streamForm").addEventListener("submit", (e) => {
        e.preventDefault()
        this.startStream()
      })
  
      // File input change
      document.getElementById("subtitleFile").addEventListener("change", (e) => {
        const fileName = e.target.files[0]?.name || "Choose File"
        const label = document.querySelector(".file-input-label")
        if (e.target.files[0]) {
          label.innerHTML = `
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                          <polyline points="14,2 14,8 20,8"></polyline>
                      </svg>
                      ${fileName}
                  `
        }
      })
    }
  
    async startStream() {
      const magnetInput = document.getElementById("magnet")
      const magnetLink = magnetInput.value.trim()
  
      if (!magnetLink) {
        this.showNotification("Please enter a magnet link", "error")
        return
      }
  
      if (!magnetLink.startsWith("magnet:?")) {
        this.showNotification("Please enter a valid magnet link", "error")
        return
      }
  
      // Auto-reset if there's an existing video to start fresh
      if (this.currentVideoUrl) {
        await this.resetSession()
        // Wait a moment for the reset to complete
        await new Promise((resolve) => setTimeout(resolve, 500))
      }
  
      const btn = document.getElementById("streamBtn")
      const btnText = document.getElementById("btnText")
      const loader = document.getElementById("btnLoader")
  
      // Update button state
      btn.disabled = true
      btnText.textContent = "Processing..."
      loader.style.display = "block"
  
      try {
        const response = await fetch(`${this.apiBase}/stream`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ magnet: magnetLink }),
        })
  
        const result = await response.json()
  
        if (result.success) {
          this.showNotification("Stream started successfully!", "success")
          this.startProgressPolling()
        } else {
          this.showNotification(result.error || "Failed to start stream", "error")
        }
      } catch (error) {
        console.error("Stream error:", error)
        this.showNotification("Network error occurred", "error")
      } finally {
        // Reset button state
        btn.disabled = false
        btnText.innerHTML = `
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <polygon points="5 3 19 12 5 21 5 3"></polygon>
                  </svg>
                  Start Streaming
              `
        loader.style.display = "none"
      }
    }
  
    async updateStatus() {
      try {
        const response = await fetch(`${this.apiBase}/status`)
        const result = await response.json()
  
        if (result.success) {
          const data = result.data
  
          // Only update UI if we have meaningful changes
          if (this.shouldUpdateUI(data)) {
            this.updateUI(data)
          }
        }
      } catch (error) {
        console.error("Status update error:", error)
      }
    }
  
    shouldUpdateUI(newData) {
      // Always update status text
      const currentStatus = document.getElementById("statusText").textContent
      if (currentStatus !== newData.status) {
        return true
      }
  
      // Update if video URL appears for the first time
      if (newData.videoUrl && !this.currentVideoUrl) {
        this.currentVideoUrl = newData.videoUrl
        return true
      }
  
      // Update if progress changed significantly (more than 1%)
      const currentProgress = Number.parseFloat(document.getElementById("progressBar")?.style.width) || 0
      if (Math.abs(currentProgress - newData.progress) > 1) {
        return true
      }
  
      // Update if subtitles changed
      if (JSON.stringify(newData.subtitles) !== JSON.stringify(this.currentSubtitles)) {
        return true
      }
  
      return false
    }
  
    updateUI(data) {
      // Update status text
      document.getElementById("statusText").textContent = data.status
  
      // Update magnet link input
      if (data.magnet) {
        document.getElementById("magnet").value = data.magnet
      }
  
      // Show/hide progress
      const progressContainer = document.getElementById("progressContainer")
      const progressText = document.getElementById("progressText")
  
      if (data.downloading) {
        progressContainer.style.display = "block"
        progressText.style.display = "block"
        document.getElementById("progressBar").style.width = `${data.progress}%`
        progressText.textContent = `${data.progress.toFixed(1)}% downloaded`
      } else {
        progressContainer.style.display = "none"
        progressText.style.display = "none"
      }
  
      // Show/hide video and file info
      const videoContainer = document.getElementById("videoContainer")
      const fileInfo = document.getElementById("fileInfo")
  
      if (data.videoUrl) {
        // Update file info
        fileInfo.style.display = "flex"
        document.getElementById("fileType").textContent = data.fileType?.toUpperCase() || "FILE"
        document.getElementById("fileSize").textContent = this.formatFileSize(data.fileSize)
  
        // Update video ONLY if URL has changed
        videoContainer.style.display = "block"
        const video = document.getElementById("videoPlayer")
  
        // Get current video URL more reliably
        const currentVideoUrl = video.currentSrc || video.src || ""
        const newVideoUrl = data.videoUrl
  
        // Only update if the URL is actually different
        if (currentVideoUrl !== newVideoUrl && !currentVideoUrl.includes(newVideoUrl)) {
          console.log("Video URL changed, updating:", { from: currentVideoUrl, to: newVideoUrl })
  
          // Set the video source directly (don't use source elements)
          video.src = newVideoUrl
  
          // Add one-time event listeners for this load
          const handleLoadStart = () => {
            console.log("Video loading started")
            video.removeEventListener("loadstart", handleLoadStart)
          }
  
          const handleCanPlay = () => {
            console.log("Video can start playing")
            video.removeEventListener("canplay", handleCanPlay)
          }
  
          const handleError = (e) => {
            console.error("Video error:", e.target.error)
            this.showNotification("Error loading video", "error")
            video.removeEventListener("error", handleError)
          }
  
          video.addEventListener("loadstart", handleLoadStart)
          video.addEventListener("canplay", handleCanPlay)
          video.addEventListener("error", handleError)
        }
  
        // Update subtitles
        this.updateSubtitles(data.subtitles || [])
      } else {
        videoContainer.style.display = "none"
        fileInfo.style.display = "none"
        document.getElementById("subtitleControls").style.display = "none"
      }
    }
  
    updateSubtitles(subtitles) {
      this.currentSubtitles = subtitles
      const subtitleControls = document.getElementById("subtitleControls")
  
      if (subtitles.length > 0) {
        subtitleControls.style.display = "flex"
  
        // Clear existing subtitle buttons (except "None")
        const existingButtons = subtitleControls.querySelectorAll(".btn:not(:first-child)")
        existingButtons.forEach((btn) => btn.remove())
  
        // Add subtitle buttons
        subtitles.forEach((subtitle) => {
          const button = document.createElement("button")
          button.className = "btn btn-secondary"
          button.onclick = () => this.setSubtitle(subtitle.path, subtitle.lang)
          button.innerHTML = `
                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z"></path>
                  </svg>
                  ${subtitle.name}
              `
          subtitleControls.appendChild(button)
        })
      } else {
        subtitleControls.style.display = "none"
      }
    }
  
    async updateProgress() {
      try {
        const response = await fetch(`${this.apiBase}/progress`)
        const result = await response.json()
  
        if (result.success) {
          const data = result.data
          const progressBar = document.getElementById("progressBar")
          const progressText = document.getElementById("progressText")
  
          if (progressBar) {
            progressBar.style.width = `${data.progress}%`
          }
          if (progressText) {
            progressText.textContent = `${data.progress.toFixed(1)}% downloaded`
          }
  
          // Stop polling if completed
          if (data.progress >= 100) {
            this.stopProgressPolling()
          }
        }
      } catch (error) {
        console.error("Progress update error:", error)
      }
    }
  
    startStatusPolling() {
      this.updateStatus() // Initial update
      this.statusInterval = setInterval(() => {
        this.updateStatus()
      }, 5000) // Changed from 2000 to 5000 (5 seconds)
    }
  
    startProgressPolling() {
      if (this.progressInterval) {
        clearInterval(this.progressInterval)
      }
  
      this.progressInterval = setInterval(() => {
        this.updateProgress()
      }, 1000)
    }
  
    stopProgressPolling() {
      if (this.progressInterval) {
        clearInterval(this.progressInterval)
        this.progressInterval = null
      }
    }
  
    setSubtitle(url, lang) {
      const video = document.getElementById("videoPlayer")
      const track = document.getElementById("subtitleTrack")
  
      if (url === "none") {
        track.src = ""
        track.label = "None"
        track.srclang = "none"
        track.mode = "disabled"
      } else {
        track.src = url
        track.label = lang || "Custom"
        track.srclang = lang || "custom"
        track.mode = "showing"
      }
  
      // Update active button
      document.querySelectorAll(".subtitle-controls .btn").forEach((btn) => {
        btn.classList.remove("active")
      })
      event.target.classList.add("active")
    }
  
    async uploadSubtitle() {
      const fileInput = document.getElementById("subtitleFile")
      if (!fileInput.files.length) {
        this.showNotification("Please select a subtitle file first", "error")
        return
      }
  
      const formData = new FormData()
      formData.append("subtitle", fileInput.files[0])
  
      try {
        const response = await fetch(`${this.apiBase}/upload-subtitle`, {
          method: "POST",
          body: formData,
        })
  
        const result = await response.json()
  
        if (result.success) {
          this.showNotification("Subtitle uploaded successfully", "success")
          // Reset file input
          fileInput.value = ""
          document.querySelector(".file-input-label").innerHTML = `
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                          <polyline points="7,10 12,15 17,10"></polyline>
                          <line x1="12" y1="15" x2="12" y2="3"></line>
                      </svg>
                      Choose File
                  `
          // Refresh status to get updated subtitles
          setTimeout(() => this.updateStatus(), 1000)
        } else {
          this.showNotification(result.error || "Upload failed", "error")
        }
      } catch (error) {
        console.error("Upload error:", error)
        this.showNotification("Upload failed", "error")
      }
    }
  
    async pasteMagnetLink() {
      try {
        const text = await navigator.clipboard.readText()
        if (text.startsWith("magnet:")) {
          document.getElementById("magnet").value = text
          this.showNotification("Magnet link pasted!", "success")
        } else {
          this.showNotification("Clipboard doesn't contain a valid magnet link", "error")
        }
      } catch (err) {
        this.showNotification("Failed to access clipboard. Make sure you're using HTTPS", "error")
        console.error("Failed to read clipboard contents: ", err)
      }
    }
  
    clearMagnetLink() {
      document.getElementById("magnet").value = ""
      document.getElementById("magnet").focus()
    }
  
    async resetSession() {
      try {
        const response = await fetch(`${this.apiBase}/reset-session`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
        })
  
        const result = await response.json()
  
        if (result.success) {
          // Clear current state
          this.currentVideoUrl = null
          this.currentSubtitles = []
  
          // Stop polling
          this.stopProgressPolling()
  
          // Reset UI
          document.getElementById("magnet").value = ""
          document.getElementById("statusText").textContent = "Ready to stream"
          document.getElementById("videoContainer").style.display = "none"
          document.getElementById("fileInfo").style.display = "none"
          document.getElementById("subtitleControls").style.display = "none"
          document.getElementById("progressContainer").style.display = "none"
          document.getElementById("progressText").style.display = "none"
  
          this.showNotification("Session reset successfully", "success")
        } else {
          this.showNotification(result.error || "Failed to reset session", "error")
        }
      } catch (error) {
        console.error("Reset session error:", error)
        this.showNotification("Failed to reset session", "error")
      }
    }
  
    showNotification(message, type) {
      const notification = document.getElementById("notification")
      const notificationText = document.getElementById("notificationText")
  
      notificationText.textContent = message
      notification.className = `notification ${type}`
      notification.classList.add("show")
  
      setTimeout(() => {
        notification.classList.remove("show")
      }, 4000)
    }
  
    formatFileSize(bytes) {
      if (!bytes) return "-"
  
      const units = ["B", "KB", "MB", "GB", "TB"]
      let size = bytes
      let unitIndex = 0
  
      while (size >= 1024 && unitIndex < units.length - 1) {
        size /= 1024
        unitIndex++
      }
  
      return `${size.toFixed(1)} ${units[unitIndex]}`
    }
  }
  
  // Global functions for HTML onclick handlers
  function pasteMagnetLink() {
    window.streamer.pasteMagnetLink()
  }
  
  function clearMagnetLink() {
    window.streamer.clearMagnetLink()
  }
  
  function setSubtitle(url, lang) {
    window.streamer.setSubtitle(url, lang)
  }
  
  function uploadSubtitle() {
    window.streamer.uploadSubtitle()
  }
  
  // Add this function with the other global functions
  function resetSession() {
    window.streamer.resetSession()
  }
  
  // Initialize the application when DOM is loaded
  document.addEventListener("DOMContentLoaded", () => {
    window.streamer = new TorrentStreamer()
  })
  