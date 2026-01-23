# K-Share: Encrypted Local Media Sharing

A high-performance, professional-grade alternative to cloud sharing. **K-Share** bridges the gap between your Android device and Windows PC with real-time synchronization, robust encryption, and a "set-and-forget" background architecture.

---

## 💎 Core Features

### ⚡ **Real-Time Clipboard**
* **Instant Sync:** Uses WebSockets to push text and links between devices in milliseconds.
* **Rich Link Support:** URLs in the clipboard and history are automatically detected and clickable on both Android and the Web Dashboard.
* **History:** Securely stores the last 20 snippets with selective deletion support.

### 📂 **Seamless File Transfer**
* **Drag & Drop:** Drop files directly into your browser to send them to your phone.
* **Smart Previews:** High-performance thumbnail generation for images with dual-layer (Memory + Disk) caching on Android.
* **WiFi-Aware:** Android transfers only occur on WiFi. 

### 🔐 **Security-First Design**
* **AES-256-GCM:** All data (clipboard, file lists, and files) is wrapped in secured encryption.
* **Zero-Knowledge:** Your "Pairing Code" never leaves your local network. It is hashed (SHA-256) locally to derive encryption keys.
* **Smart Diagnostics:** Android app provides specific connection error messages (e.g., "Connection Refused", "Decryption Failed") for easy troubleshooting.
* **Replay Protection:** Encrypted payloads include UTC timestamps to prevent intercepted message re-injection.

### 🖱️ **Desktop Integration**
* **System Tray:** Runs silently in the Windows tray. Right-click to open the dashboard or exit.
* **Auto-Start:** Simply type `shell:startup` in the Run dialog (`Win+R`) and paste a shortcut to `k-share.exe` to have it start with Windows.
* **Open on PC:** One-tap from the Android share menu to instantly launch a URL in your laptop's default browser.

---

## 🛠️ Compilation Guide (Windows Server)

You can compile the Go server into a single executable using one of two methods. These commands use optimization flags to produce the smallest possible binary.

### **Method 1: Console Mode (With Terminal)**
Use this for initial setup or debugging. This version will keep a Command Prompt window open while running.
```bash
cd windows-server
go build -trimpath -ldflags="-s -w" -o k-share.exe
```

### **Method 2: Background Mode (Hidden Window)**
Use this for daily usage. The server will start silently, and you will only see the **K-Share icon** in your system tray.
```bash
cd windows-server
go build -trimpath -ldflags="-s -w -H windowsgui" -o k-share.exe
```

---

## ⚙️ Configuration

The server is controlled by `config.json` in the `windows-server` directory. 

> **Quick Start:** A template file `config.example.json` is provided. Rename it to `config.json` and fill in your details.

| Field | Description |
| :--- | :--- |
| `port` | The local port to run on (default: `26260`). |
| `pairing_code` | Your private password. Must match on both PC and Phone. |
| `github_token` | GitHub PAT for Gist-based discovery. |
| `gist_id` | The ID of your private secret Gist. |
| `gist_filename` | The name of the file inside the Gist (e.g., `server.json`). |
| `to_phone_dir` | Local folder for files being sent to the phone. |
| `from_phone_dir` | Local folder for files uploaded from the phone. |

---

## 🌍 Zero-Config Discovery (GitHub Gists)

To avoid manually typing your laptop's IP address every time your router assigns a new one (DHCP), K-Share uses a "Dead Drop" discovery mechanism via GitHub Gists.

### **🔄 How it Works**
1. **Server-Side (Push):** Every 2 minutes, the Windows server checks its local IP. If it changes, it automatically performs an encrypted `PATCH` request to your private GitHub Gist with the new address.
2. **Android-Side (Pull):** When you tap **Re-Discover** (or every 15 minutes via background WorkManager), the app fetches the Gist content and updates its internal connection settings.
3. **Resilience:** The server includes **smart offline detection**, capable of identifying your local LAN IP even without an active internet connection, ensuring manual pairing always works.

### **1. Setup a Gist**
1. Go to [gist.github.com](https://gist.github.com).
2. Create a **new Secret Gist**.
3. Name the file `server.json`.
4. Add this content: `{"ip": "0.0.0.0"}`
5. Click **Create secret gist**.
6. **Important URL:** Click the **"Raw"** button on your new Gist. Copy this URL but **remove the specific commit hash** (the part between your username and the filename) so it always points to the latest version.
   * *Correct:* `https://gist.githubusercontent.com/username/gist_id/raw/server.json`
   * *Incorrect:* `https://gist.githubusercontent.com/username/gist_id/raw/LONG_HASH/server.json`

### **2. Configure Windows Server**
Edit `config.json` in the `windows-server` folder:
* `gist_id`: The ID from your Gist URL.
* `github_token`: Create a [GitHub Classic Token](https://github.com/settings/tokens) with the `gist` scope.
* `gist_filename`: Must match the filename you used (e.g., `ip.json`).

### **3. Configure Android App**
1. Tap **Config** in the app.
2. **Gist Raw URL**: Paste the "cleaned" Raw URL from Step 1.
3. **JSON Key**: Set to `ip`.
4. Tap **Save Settings** and then **Re-Discover**.


> [!NOTE]
> **Why not Tailscale or mDNS?**
> Traditional discovery methods like mDNS (multicast) and Tailscale were intentionally avoided to ensure maximum compatibility with restrictive environments, such as university or corporate WiFi networks. These networks frequently block peer-to-peer discovery protocols and proprietary tunneling. K-Share's Gist-based "Dead Drop" system and direct WebSocket connections provide a robust alternative that works over standard ports.

---

## 📱 Android Setup

1. Open the project in **Android Studio**.
2. Perform a **Build > Clean Project** then **Build > Rebuild Project**.
3. Install the APK on your device.
4. In **Settings**, enter your **Pairing Code** and **Gist URL**.
5. Enable **WiFi** to begin syncing.

---

## 🛰️ Technical Stack

* **Backend:** Go (Gorilla WebSockets, Systray, Resize Library).
* **Web:** Vanilla JavaScript, Web Crypto API (SubtleCrypto).
* **Mobile:** Kotlin, Jetpack Compose, WorkManager, OkHttp, LruCache.
* **Discovery:** Automated discovery via GitHub Gists (fallback to manual IP).

---

## 📄 License
This project is private and intended for personal use. Maintain your `pairing_code` and `github_token` securely.
