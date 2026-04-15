package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Data structures mapping the provided JSON payload
type Artist struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Track struct {
	ID    int      `json:"id"`
	Title string   `json:"title"`
	Image string   `json:"image"`
	Artists []Artist `json:"artists"`
}

type NowPlayingResponse struct {
	CurrentlyPlaying bool   `json:"currently_playing"`
	Track            Track  `json:"track"`
}

var (
	koitoAddress string
	currentData  NowPlayingResponse
	dataMutex    sync.RWMutex
)

func main() {
	koitoAddress = os.Getenv("KOITO_ADDRESS")
	if koitoAddress == "" {
		log.Fatal("Error: KOITO_ADDRESS environment variable is required (e.g., http://localhost:9000)")
	}
	// Trim trailing slash for clean URL concatenation later
	koitoAddress = strings.TrimRight(koitoAddress, "/")

	// Start the background poller
	go pollEndpoint()

	// Register HTTP handlers
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/state", stateHandler)
	http.HandleFunc("/image/", imageProxyHandler)

	log.Println("Server listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// pollEndpoint queries the API once every second and caches the result
func pollEndpoint() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	client := &http.Client{Timeout: 2 * time.Second}

	for range ticker.C {
		resp, err := client.Get(koitoAddress + "/apis/web/v1/now-playing")
		if err != nil {
			log.Printf("Error polling endpoint: %v", err)
			continue
		}

		var data NowPlayingResponse
		err = json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		
		if err != nil {
			log.Printf("Error decoding JSON: %v", err)
			continue
		}

		// Update the shared state safely
		dataMutex.Lock()
		currentData = data
		dataMutex.Unlock()
	}
}

// stateHandler serves the latest cached track data to the frontend
func stateHandler(w http.ResponseWriter, r *http.Request) {
	dataMutex.RLock()
	defer dataMutex.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(currentData)
}

// imageProxyHandler proxies the album art to prevent CORS issues in the browser
func imageProxyHandler(w http.ResponseWriter, r *http.Request) {
	imgUUID := strings.TrimPrefix(r.URL.Path, "/image/")
	if imgUUID == "" {
		http.NotFound(w, r)
		return
	}
	
	targetURL := fmt.Sprintf("%s/images/large/%s", koitoAddress, imgUUID)
	resp, err := http.Get(targetURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		http.Error(w, "Image not found", http.StatusNotFound)
		if resp != nil {
			resp.Body.Close()
		}
		return
	}
	defer resp.Body.Close()
	
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	io.Copy(w, resp.Body)
}

// indexHandler serves the frontend HTML, CSS, and JS
func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlPage))
}

const htmlPage = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Now Playing Widget</title>
    <style>
        body {
            /* Pure green screen background */
            background-color: #00FF00; 
            color: white;
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            margin: 0;
            padding: 20px;
            display: flex;
            align-items: center;
            height: 100vh;
            box-sizing: border-box;
            /* Text shadow to ensure readability over video inputs */
            text-shadow: 2px 2px 4px rgba(0,0,0,0.8);
        }
        .container {
            display: flex;
            align-items: center;
            gap: 20px;
            background: rgba(0, 0, 0, 0.6);
            padding: 15px 25px 15px 15px;
            border-radius: 12px;
            transition: opacity 0.3s ease;
        }
        #album-art {
            width: 120px;
            height: 120px;
            border-radius: 8px;
            object-fit: cover;
            box-shadow: 0 4px 8px rgba(0,0,0,0.5);
        }
        .info {
            display: flex;
            flex-direction: column;
        }
        #title {
            font-size: 2em;
            font-weight: bold;
            margin: 0 0 5px 0;
        }
        #artist {
            font-size: 1.3em;
            margin: 0;
            color: #dcdcdc;
        }
    </style>
</head>
<body>
    <div class="container" id="now-playing-container" style="opacity: 0;">
        <img id="album-art" src="" alt="Album Art">
        <div class="info">
            <p id="title"></p>
            <p id="artist"></p>
        </div>
    </div>

    <script>
        async function fetchState() {
            try {
                const response = await fetch('/state');
                const data = await response.json();
                
                const container = document.getElementById('now-playing-container');
                const img = document.getElementById('album-art');
                const title = document.getElementById('title');
                const artist = document.getElementById('artist');

                if (data.currently_playing && data.track) {
                    container.style.opacity = '1';
                    
                    // Only update the image if the source actually changed to prevent flickering
                    const newImgSrc = '/image/' + data.track.image;
                    if (!img.src.endsWith(newImgSrc)) {
                        img.src = newImgSrc;
                    }

                    title.innerText = data.track.title;
                    
                    // Join multiple artists with a comma if they exist
                    if (data.track.artists && data.track.artists.length > 0) {
                        artist.innerText = data.track.artists.map(a => a.name).join(', ');
                    } else {
                        artist.innerText = 'Unknown Artist';
                    }
                } else {
                    // Hide container gracefully if nothing is playing
                    container.style.opacity = '0';
                }
            } catch (error) {
                console.error("Failed to fetch state:", error);
            }
        }

        // Fetch immediately, then poll our internal state route every 1000ms
        fetchState();
        setInterval(fetchState, 1000);
    </script>
</body>
</html>
`
