class TorrentStreamer {
    constructor() {
      this.apiBase = "/api"
      this.ytsApiBase = "https://yts.mx/api/v2"
      this.currentSubtitles = []
      this.progressInterval = null
      this.statusInterval = null
      this.currentVideoUrl = null
      this.startTime = Date.now()
      this.currentPage = 1
      this.currentMovies = []
      this.featuredMovies = []
      this.currentFilters = {
        query_term: "",
        genre: "",
        quality: "",
        minimum_rating: "0",
        sort_by: "date_added",
        order_by: "desc",
      }
      this.gridView = "grid"
  
      this.initializeEventListeners()
      this.initializeHeader()
      this.startStatusPolling()
      this.loadMovies()
      this.loadFeaturedMovies()
    }
  
    initializeEventListeners() {
      // Form submission
      document.getElementById("streamForm").addEventListener("submit", (e) => {
        e.preventDefault()
        this.startStream()
      })
  
      // File input change
      document.getElementById("subtitleFile").addEventListener("change", (e) => {
        const fileName = e.target.files[0]?.name || "Choose Subtitle File"
        const label = document.querySelector(".file-input-label span")
        if (e.target.files[0]) {
          label.textContent = fileName
        } else {
          label.textContent = "Upload Custom Subtitles"
        }
      })
  
      // Video time update
      const video = document.getElementById("videoPlayer")
      video.addEventListener("timeupdate", () => {
        this.updateStreamTime()
      })
  
      // Search inputs
      document.getElementById("movieSearch")?.addEventListener("keypress", (e) => {
        if (e.key === "Enter") {
          this.searchMovies()
        }
      })
  
      document.getElementById("headerSearch")?.addEventListener("keypress", (e) => {
        if (e.key === "Enter") {
          this.headerSearch()
        }
      })
  
      // Header scroll effect
      window.addEventListener("scroll", () => {
        this.handleHeaderScroll()
      })
  
      // Hero background rotation
      this.startHeroRotation()
    }
  
    initializeHeader() {
      const header = document.querySelector(".header")
      if (header) {
        // Add scroll effect
        this.handleHeaderScroll()
      }
    }
  
    handleHeaderScroll() {
      const header = document.querySelector(".header")
      if (window.scrollY > 100) {
        header.classList.add("scrolled")
      } else {
        header.classList.remove("scrolled")
      }
    }
  
    startHeroRotation() {
      // Rotate hero background every 10 seconds
      setInterval(() => {
        this.updateHeroBackground()
      }, 10000)
    }
  
    async updateHeroBackground() {
      if (this.featuredMovies.length > 0) {
        const randomMovie = this.featuredMovies[Math.floor(Math.random() * this.featuredMovies.length)]
        this.setHeroMovie(randomMovie)
      }
    }
  
    setHeroMovie(movie) {
      const heroImage = document.getElementById("heroImage")
      const heroTitle = document.getElementById("heroTitle")
      const heroDescription = document.getElementById("heroDescription")
      const heroPlayBtn = document.getElementById("heroPlayBtn")
  
      if (heroImage && movie.background_image_original) {
        heroImage.src = movie.background_image_original
        heroImage.alt = movie.title
      }
  
      if (heroTitle) {
        heroTitle.textContent = movie.title_long || movie.title
      }
  
      if (heroDescription) {
        const description =
          movie.summary || movie.description_full || "Discover amazing movies and stream them instantly in HD quality."
        heroDescription.textContent = description.length > 200 ? description.substring(0, 200) + "..." : description
      }
  
      if (heroPlayBtn) {
        heroPlayBtn.onclick = () => this.showMovieDetails(movie.id)
      }
    }
  
    async loadFeaturedMovies() {
      try {
        const response = await fetch(`${this.ytsApiBase}/list_movies.json?limit=10&minimum_rating=8&sort_by=likes`)
        const result = await response.json()
  
        if (result.status && result.data.movies) {
          this.featuredMovies = result.data.movies
          if (this.featuredMovies.length > 0) {
            this.setHeroMovie(this.featuredMovies[0])
          }
        }
      } catch (error) {
        console.error("Error loading featured movies:", error)
      }
    }
  
    // Navigation
    showSection(sectionName) {
      // Update navigation
      document.querySelectorAll(".nav-link").forEach((link) => {
        link.classList.remove("active")
        if (link.dataset.section === sectionName) {
          link.classList.add("active")
        }
      })
  
      // Show/hide sections
      document.querySelectorAll(".content-section").forEach((section) => {
        section.classList.remove("active")
      })
  
      const targetSection = document.getElementById(sectionName + "Section")
      if (targetSection) {
        targetSection.classList.add("active")
        targetSection.classList.add("fade-in")
      }
  
      // Auto-switch to player when streaming starts
      if (sectionName === "player") {
        this.updateStatus()
      }
  
      // Scroll to top
      window.scrollTo({ top: 0, behavior: "smooth" })
    }
  
    scrollToMovies() {
      const moviesSection = document.getElementById("moviesSection")
      if (moviesSection) {
        moviesSection.scrollIntoView({ behavior: "smooth" })
      }
    }
  
    setGridView(viewType) {
      this.gridView = viewType
  
      // Update view toggle buttons
      document.querySelectorAll(".view-btn").forEach((btn) => {
        btn.classList.remove("active")
      })
      event.target.classList.add("active")
  
      // Update grid class
      const moviesGrid = document.getElementById("moviesGrid")
      if (moviesGrid) {
        moviesGrid.className = viewType === "list" ? "movies-list" : "movies-grid"
      }
  
      // Re-render movies with new view
      this.displayMovies(this.currentMovies)
    }
  
    // YTS Movie Functions
    async loadMovies(page = 1) {
      this.currentPage = page
      this.showMoviesLoading(true)
  
      try {
        const params = new URLSearchParams({
          page: page.toString(),
          limit: "20",
          ...this.currentFilters,
        })
  
        const response = await fetch(`${this.ytsApiBase}/list_movies.json?${params}`)
        const result = await response.json()
  
        if (result.status=="ok") {
          this.currentMovies = result.data.movies || []
          this.displayMovies(this.currentMovies)
          this.updatePagination(result.data.page_number, result.data.movie_count)
        } else {
          this.showNotification("Failed to load movies", "error")
        }
      } catch (error) {
        console.error("Error loading movies:", error)
        this.showNotification("Error loading movies", "error")
      } finally {
        this.showMoviesLoading(false)
      }
    }
  
    displayMovies(movies) {
      const grid = document.getElementById("moviesGrid")
  
      if (!movies || movies.length === 0) {
        grid.innerHTML = `
          <div class="no-movies-found">
            <div class="no-movies-icon">
              <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
                <circle cx="11" cy="11" r="8"></circle>
                <path d="m21 21-4.35-4.35"></path>
              </svg>
            </div>
            <h3>No movies found</h3>
            <p>Try adjusting your search or filters</p>
            <button onclick="clearFilters()" class="btn btn-primary" style="margin-top: 1rem;">
              Clear Filters
            </button>
          </div>
        `
        return
      }
  
      if (this.gridView === "list") {
        grid.innerHTML = movies
          .map(
            (movie) => `
          <div class="movie-list-item" onclick="showMovieDetails(${movie.id})">
            <div class="movie-list-poster">
              <img src="${movie.medium_cover_image}" alt="${movie.title}" loading="lazy">
              <div class="movie-quality">${this.getBestQuality(movie.torrents)}</div>
            </div>
            <div class="movie-list-info">
              <h3 class="movie-title">${movie.title}</h3>
              <div class="movie-meta">
                <span class="movie-year">${movie.year}</span>
                <span class="movie-rating">
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
                    <polygon points="12,2 15.09,8.26 22,9.27 17,14.14 18.18,21.02 12,17.77 5.82,21.02 7,14.14 2,9.27 8.91,8.26"></polygon>
                  </svg>
                  ${movie.rating}
                </span>
                <span>${movie.runtime} min</span>
              </div>
              <div class="movie-genres">
                ${movie.genres
                  .slice(0, 3)
                  .map((genre) => `<span class="movie-genre">${genre}</span>`)
                  .join("")}
              </div>
              <p class="movie-summary">${(movie.summary || "").substring(0, 150)}${movie.summary && movie.summary.length > 150 ? "..." : ""}</p>
            </div>
            <div class="movie-list-actions">
              <button class="stream-btn" onclick="event.stopPropagation(); quickStream(${movie.id})">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M8 5v14l11-7z"/>
                </svg>
                Stream
              </button>
            </div>
          </div>
        `,
          )
          .join("")
      } else {
        grid.innerHTML = movies
          .map(
            (movie) => `
          <div class="movie-card" onclick="showMovieDetails(${movie.id})">
            <div class="movie-poster">
              <img src="${movie.medium_cover_image}" alt="${movie.title}" loading="lazy">
              <div class="movie-quality">${this.getBestQuality(movie.torrents)}</div>
              <div class="movie-rating">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
                  <polygon points="12,2 15.09,8.26 22,9.27 17,14.14 18.18,21.02 12,17.77 5.82,21.02 7,14.14 2,9.27 8.91,8.26"></polygon>
                </svg>
                ${movie.rating}
              </div>
              <div class="movie-overlay">
                <button class="quick-play-btn" onclick="event.stopPropagation(); quickStream(${movie.id})">
                  <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M8 5v14l11-7z"/>
                  </svg>
                </button>
              </div>
            </div>
            <div class="movie-info">
              <h3 class="movie-title">${movie.title}</h3>
              <div class="movie-meta">
                <span class="movie-year">${movie.year}</span>
                <span>${movie.runtime} min</span>
              </div>
              <div class="movie-genres">
                ${movie.genres
                  .slice(0, 3)
                  .map((genre) => `<span class="movie-genre">${genre}</span>`)
                  .join("")}
              </div>
            </div>
          </div>
        `,
          )
          .join("")
      }
  
      // Add fade-in animation
      grid.classList.add("fade-in")
    }
  
    getBestQuality(torrents) {
      if (!torrents || torrents.length === 0) return "N/A"
  
      const qualityOrder = ["2160p", "1080p", "720p", "480p"]
      for (const quality of qualityOrder) {
        if (torrents.some((t) => t.quality === quality)) {
          return quality
        }
      }
      return torrents[0].quality
    }
  
    showMoviesLoading(show) {
      const loading = document.getElementById("moviesLoading")
      const grid = document.getElementById("moviesGrid")
  
      if (show) {
        loading.style.display = "flex"
        grid.style.display = "none"
      } else {
        loading.style.display = "none"
        grid.style.display = this.gridView === "list" ? "flex" : "grid"
        grid.style.flexDirection = this.gridView === "list" ? "column" : "initial"
      }
    }
  
    updatePagination(currentPage, totalMovies) {
      const pageInfo = document.getElementById("pageInfo")
      const prevBtn = document.getElementById("prevPage")
      const nextBtn = document.getElementById("nextPage")
  
      const totalPages = Math.ceil(totalMovies / 20)
  
      pageInfo.textContent = `Page ${currentPage} of ${totalPages}`
      prevBtn.disabled = currentPage <= 1
      nextBtn.disabled = currentPage >= totalPages
    }
  
    changePage(direction) {
      const newPage = this.currentPage + direction
      if (newPage >= 1) {
        this.loadMovies(newPage)
        this.scrollToMovies()
      }
    }
  
    searchMovies() {
      const searchInput = document.getElementById("movieSearch")
      this.currentFilters.query_term = searchInput.value.trim()
      this.loadMovies(1)
    }
  
    headerSearch() {
      const searchInput = document.getElementById("headerSearch")
      const query = searchInput.value.trim()
  
      if (query) {
        // Update the main search and switch to browse section
        //document.getElementById("movieSearch").value = query
        this.currentFilters.query_term = query
        //this.showSection("moviesSection")
        document.getElementById('moviesSection').scrollIntoView({ 
            behavior: 'smooth' 
          });
        this.loadMovies(1)
  
      
      }
    }
  
    filterMovies() {
      this.currentFilters.genre = document.getElementById("genreFilter").value
      this.currentFilters.quality = document.getElementById("qualityFilter").value
      this.currentFilters.minimum_rating = document.getElementById("ratingFilter").value
      this.currentFilters.sort_by = document.getElementById("sortFilter").value
      this.loadMovies(1)
    }
  
    clearFilters() {
      this.currentFilters = {
        query_term: "",
        genre: "",
        quality: "",
        minimum_rating: "0",
        sort_by: "date_added",
        order_by: "desc",
      }
  
      // Reset form elements
      document.getElementById("movieSearch").value = ""
      document.getElementById("genreFilter").value = ""
      document.getElementById("qualityFilter").value = ""
      document.getElementById("ratingFilter").value = "0"
      document.getElementById("sortFilter").value = "date_added"
  
      this.loadMovies(1)
    }
  
    async quickStream(movieId) {
      try {
        const response = await fetch(`${this.ytsApiBase}/movie_details.json?movie_id=${movieId}`)
        const result = await response.json()
  
        if (result.status=="ok" && result.data.torrents && result.data.torrents.length > 0) {
          // Get the best quality torrent
          const bestTorrent = this.getBestQualityTorrent(result.data.torrents)
          await this.streamMovie(movieId, bestTorrent.hash, bestTorrent.quality, result.data.title)
        } else {
          this.showNotification("No torrents available for this movie", "error")
        }
      } catch (error) {
        console.error("Quick stream error:", error)
        this.showNotification("Error starting quick stream", "error")
      }
    }
  
    getBestQualityTorrent(torrents) {
      const qualityOrder = ["1080p", "720p", "2160p", "480p"]
  
      for (const quality of qualityOrder) {
        const torrent = torrents.find((t) => t.quality === quality)
        if (torrent) return torrent
      }
  
      return torrents[0] // fallback to first available
    }
  
    async showMovieDetails(movieId) {
      try {
        // Show loading state
        this.showMovieModal({ title: "Loading...", loading: true })
  
        const response = await fetch(`${this.ytsApiBase}/movie_details.json?movie_id=${movieId}`)
        const result = await response.json()
  
        if (result.status=="ok") {
          this.displayMovieModal(result.data)
        } else {
          this.showNotification("Failed to load movie details", "error")
          this.closeMovieModal()
        }
      } catch (error) {
        console.error("Error loading movie details:", error)
        this.showNotification("Error loading movie details", "error")
        this.closeMovieModal()
      }
    }
  
    showMovieModal(movie) {
      const modal = document.getElementById("movieModal")
      modal.classList.add("active")
  
      if (movie.loading) {
        document.getElementById("modalTitle").textContent = movie.title
        document.getElementById("modalPoster").src = "/placeholder.svg?height=450&width=300"
        return
      }
    }
  
    displayMovieModal(movie) {
      movie=movie.movie
      const modal = document.getElementById("movieModal")
  
      document.getElementById("modalTitle").textContent = movie.title_long || movie.title
      document.getElementById("modalPoster").src = movie.large_cover_image
      document.getElementById("modalPoster").alt = movie.title
  
      document.getElementById("modalYear").textContent = movie.year
      document.getElementById("modalRating").innerHTML = `
        <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
          <polygon points="12,2 15.09,8.26 22,9.27 17,14.14 18.18,21.02 12,17.77 5.82,21.02 7,14.14 2,9.27 8.91,8.26"></polygon>
        </svg>
        ${movie.rating}/10
      `
      document.getElementById("modalRuntime").textContent = `${movie.runtime} min`
  
      document.getElementById("modalGenres").innerHTML = movie.genres
        .map((genre) => `<span class="movie-genre">${genre}</span>`)
        .join("")
  
      document.getElementById("modalSummary").textContent =
        movie.summary || movie.description_full || "No description available."
  
      const torrentsHtml = movie.torrents
        .sort((a, b) => {
          const qualityOrder = { "2160p": 4, "1080p": 3, "720p": 2, "480p": 1 }
          return (qualityOrder[b.quality] || 0) - (qualityOrder[a.quality] || 0)
        })
        .map(
          (torrent) => `
        <div class="torrent-option">
          <div class="torrent-info">
            
            <div class="torrent-details">
              <div class="torrent-size">${torrent.size}</div>
              <div>
                <span class="torrent-seeds">↑${torrent.seeds}</span> / 
                <span class="torrent-peers">↓${torrent.peers}</span>
              </div>
            </div>
          </div>
          <button class="stream-btn" onclick="streamMovie(${movie.id}, '${torrent.hash}', '${torrent.quality}', '${movie.title.replace(/'/g, "\\'")}')">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <path d="M8 5v14l11-7z"/>
            </svg>
            Stream ${torrent.quality}
          </button>
        </div>
      `,
        )
        .join("")
  
      document.getElementById("modalTorrents").innerHTML = torrentsHtml
  
      modal.classList.add("active")
    }
  
    closeMovieModal() {
      document.getElementById("movieModal").classList.remove("active")
    }
  
    async streamMovie(movieId, hash, quality, title) {
       
        let magneticLink =`magnet:?xt=urn:btih:${hash}&dn=${title}&tr=http://track.one:1234/announce&tr=udp://track.two:80`
      try {
        const response = await fetch(`${this.apiBase}/stream`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            movie_id: movieId,
            hash: hash,
            quality: quality,
            title: title,
            magnet:magneticLink
          }),
        })
  
        const result = await response.json()
        let statusText = document.getElementById("statusText")
  
        if (result.success) {
            
          statusText.textContent="Processing..."
          this.closeMovieModal()
          this.showSection("player")
          this.showNotification(`🎬 Starting ${title} (${quality})`, "success")
          this.startProgressPolling()
        } else {
          this.showNotification(result.error || "Failed to start movie stream", "error")
        }
      } catch (error) {
        console.error("Stream movie error:", error)
        statusText.textContent="Stream movie error:"
        this.showNotification("Network error occurred", "error")
      }
    }
  
    // Existing streaming functions
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
  
      if (this.currentVideoUrl) {
        await this.resetSession()
        await new Promise((resolve) => setTimeout(resolve, 500))
      }
  
      const btn = document.getElementById("streamBtn")
      const btnText = document.getElementById("btnText")
      const loader = document.getElementById("btnLoader")
  
      btn.disabled = true
      btnText.innerHTML = `
        <div class="loading-dots">
          <span></span>
          <span></span>
          <span></span>
        </div>
        Processing...
      `
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
          this.showSection("player")
          this.showNotification("🚀 Stream started successfully!", "success")
          this.startProgressPolling()
        } else {
          this.showNotification(result.error || "Failed to start stream", "error")
        }
      } catch (error) {
        console.error("Stream error:", error)
        this.showNotification("Network error occurred", "error")
      } finally {
        btn.disabled = false
        btnText.innerHTML = `
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
            <path d="M8 5v14l11-7z"/>
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
  
          if (this.shouldUpdateUI(data)) {
            this.updateUI(data)
          }
        }
      } catch (error) {
        console.error("Status update error:", error)
      }
    }
  
    shouldUpdateUI(newData) {
      const currentStatus = document.getElementById("statusText").textContent
      if (currentStatus !== newData.status) {
        return true
      }
  
      if (newData.videoUrl && !this.currentVideoUrl) {
        this.currentVideoUrl = newData.videoUrl
        return true
      }
  
      const currentProgress = Number.parseFloat(document.getElementById("progressBar")?.style.width) || 0
      if (Math.abs(currentProgress - newData.progress) > 1) {
        return true
      }
  
      if (JSON.stringify(newData.subtitles) !== JSON.stringify(this.currentSubtitles)) {
        return true
      }
  
      return false
    }
  
    updateUI(data) {
      const statusText = document.getElementById("statusText")
      const statusDot = document.querySelector(".status-dot")
  
      // Update status text
      statusText.textContent = data.status
  
      // Update status indicator color
      if (statusDot) {
        statusDot.className = "status-dot"
        if (data.status.includes("Ready")) {
          statusDot.style.background = "var(--success)"
        } else if (data.status.includes("Connecting") || data.status.includes("Fetching")) {
          statusDot.style.background = "var(--warning)"
        } else if (data.status.includes("Streaming")) {
          statusDot.style.background = "var(--primary)"
        } else if (data.status.includes("Error")) {
          statusDot.style.background = "var(--error)"
        }
      }
  
   
    
  
      const mediaSection = document.getElementById("mediaSection")
  
      if (data.videoUrl) {
        mediaSection.style.display = "block"
  
        document.getElementById("fileType").textContent = data.fileType?.toUpperCase() || "FILE"
        document.getElementById("fileSize").textContent = this.formatFileSize(data.fileSize)
        document.getElementById("quality").textContent = this.getQualityFromSize(data.fileSize)
  
        const video = document.getElementById("videoPlayer")
        const currentVideoUrl = video.currentSrc || video.src || ""
        const newVideoUrl = data.videoUrl
  
        if (currentVideoUrl !== newVideoUrl && !currentVideoUrl.includes(newVideoUrl)) {
          console.log("🎥 Updating video source:", { from: currentVideoUrl, to: newVideoUrl })
  
          video.src = newVideoUrl
  
          const handleLoadStart = () => {
            console.log("📺 Video loading started")
            video.removeEventListener("loadstart", handleLoadStart)
          }
  
          const handleCanPlay = () => {
            console.log("▶️ Video ready to play")
            video.removeEventListener("canplay", handleCanPlay)
          }
  
          const handleError = (e) => {
            console.error("❌ Video error:", e.target.error)
            this.showNotification("Error loading video", "error")
            video.removeEventListener("error", handleError)
          }
  
          video.addEventListener("loadstart", handleLoadStart)
          video.addEventListener("canplay", handleCanPlay)
          video.addEventListener("error", handleError)
        }
  
        this.updateSubtitles(data.subtitles || [])
      } else {
        mediaSection.style.display = "none"
      }
    }
  
    updateSubtitles(subtitles) {
      this.currentSubtitles = subtitles
      const subtitleControls = document.getElementById("subtitleControls")
  
      // Remove existing subtitle buttons (keep the "None" button)
      const existingButtons = subtitleControls.querySelectorAll(".subtitle-btn:not(:first-child)")
      existingButtons.forEach((btn) => btn.remove())
  
      subtitles.forEach((subtitle) => {
        const button = document.createElement("button")
        button.className = "subtitle-btn"
        button.onclick = () => this.setSubtitle(subtitle.path, subtitle.lang)
  
        const langFlag = this.getLanguageFlag(subtitle.lang)
        button.innerHTML = `
          ${langFlag}
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z"></path>
          </svg>
          ${subtitle.name}
        `
        subtitleControls.appendChild(button)
      })
    }
  
    getLanguageFlag(lang) {
      const flags = {
        en: "🇺🇸",
        fr: "🇫🇷",
        es: "🇪🇸",
        de: "🇩🇪",
        ja: "🇯🇵",
        zh: "🇨🇳",
        ko: "🇰🇷",
        ru: "🇷🇺",
        it: "🇮🇹",
        pt: "🇵🇹",
        nl: "🇳🇱",
      }
      return flags[lang] || "🌐"
    }
  
    getQualityFromSize(bytes) {
      if (!bytes) return "SD"
      const gb = bytes / (1024 * 1024 * 1024)
      if (gb > 8) return "4K"
      if (gb > 4) return "FHD"
      if (gb > 2) return "HD"
      return "SD"
    }
  
    updateStreamTime() {
      const video = document.getElementById("videoPlayer")
      if (video && video.duration) {
        const current = this.formatTime(video.currentTime)
        const total = this.formatTime(video.duration)
        document.getElementById("streamTime").textContent = `${current} / ${total}`
      }
    }
  
    formatTime(seconds) {
      const hours = Math.floor(seconds / 3600)
      const minutes = Math.floor((seconds % 3600) / 60)
      const secs = Math.floor(seconds % 60)
  
      if (hours > 0) {
        return `${hours}:${minutes.toString().padStart(2, "0")}:${secs.toString().padStart(2, "0")}`
      }
      return `${minutes}:${secs.toString().padStart(2, "0")}`
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
            progressText.textContent = `${data.progress.toFixed(1)}%`
          }
  
          if (data.progress >= 100) {
            this.stopProgressPolling()
          }
        }
      } catch (error) {
        console.error("Progress update error:", error)
      }
    }
  
    startStatusPolling() {
      this.updateStatus()
      this.statusInterval = setInterval(() => {
        this.updateStatus()
      }, 5000)
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
      document.querySelectorAll(".subtitle-btn").forEach((btn) => {
        btn.classList.remove("active")
      })
      event.target.classList.add("active")
  
      this.showNotification(`Subtitle changed to ${lang || "Custom"}`, "success")
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
          this.showNotification("📁 Subtitle uploaded successfully", "success")
          fileInput.value = ""
          document.querySelector(".file-input-label span").textContent = "Upload Custom Subtitles"
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
          this.showNotification("🔗 Magnet link pasted!", "success")
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
          this.currentVideoUrl = null
          this.currentSubtitles = []
  
          this.stopProgressPolling()
  
          document.getElementById("magnet").value = ""
          document.getElementById("statusText").textContent = "Ready to stream"
          document.getElementById("mediaSection").style.display = "none"
        
  
          // Reset status dot
          const statusDot = document.querySelector(".status-dot")
          if (statusDot) {
            statusDot.style.background = "var(--success)"
          }
  
          this.showNotification("🔄 On New Session ", "success")
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
      const notificationIcon = document.querySelector(".notification-icon")
  
      notificationText.textContent = message
      notification.className = `notification ${type}`
  
      // Update icon based on type
      if (type === "success") {
        notificationIcon.innerHTML = `
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="20,6 9,17 4,12"></polyline>
          </svg>
        `
      } else if (type === "error") {
        notificationIcon.innerHTML = `
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"></circle>
            <line x1="15" y1="9" x2="9" y2="15"></line>
            <line x1="9" y1="9" x2="15" y2="15"></line>
          </svg>
        `
      }
  
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
  function showSection(sectionName) {
    window.streamer.showSection(sectionName)
  }
  
  function showMovieDetails(movieId) {
    window.streamer.showMovieDetails(movieId)
  }
  
  function closeMovieModal() {
    window.streamer.closeMovieModal()
  }
  
  function streamMovie(movieId, hash, quality, title) {
    window.streamer.streamMovie(movieId, hash, quality, title)
  }
  
  function quickStream(movieId) {
    window.streamer.quickStream(movieId)
  }
  
  function changePage(direction) {
    window.streamer.changePage(direction)
  }
  
  function searchMovies() {
    window.streamer.searchMovies()
  }
  
  function headerSearch() {
    window.streamer.headerSearch()
  }
  
  function filterMovies() {
    window.streamer.filterMovies()
  }
  
  function clearFilters() {
    window.streamer.clearFilters()
  }
  
  function setGridView(viewType) {
    window.streamer.setGridView(viewType)
  }
  
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
  
  function resetSession() {
    window.streamer.resetSession()
  }
  
  function scrollToMovies() {
    window.streamer.scrollToMovies()
  }
  
  // Initialize the application when DOM is loaded
  document.addEventListener("DOMContentLoaded", () => {
    window.streamer = new TorrentStreamer()
  })
  