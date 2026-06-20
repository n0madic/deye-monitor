# Deye Monitor — build orchestration.
#
#   TUI  (cmd/tui)      — terminal app; pure Go, built for the host OS only.
#   GUI  (cmd/gui) — Fyne app; packaged per platform.
#
# The desktop GUI links cgo + system GL, so the native targets (gui, gui-darwin,
# gui-linux, gui-windows) only work when the host OS matches the target. To
# cross-build the desktop GUI from any host, use the fyne-cross targets (Docker).
# Android has its own pipeline (target-SDK patch + apksigner re-sign) in
# cmd/gui/build-android.sh.
#
# GUI desktop/iOS metadata (name, app-id, icon, version) is read from
# cmd/gui/FyneApp.toml, so most targets need no extra flags.
#
# Run `make help` for the target list.

# --- Configuration -----------------------------------------------------------
APP_NAME := deye-monitor
APP_ID   := com.deye.monitor
TUI_PKG  := ./cmd/tui
GUI_PKG  := ./cmd/gui
GUI_DIR  := cmd/gui
ICON     := Icon.png
BIN      := bin

# fyne package names the artifact after Details.Name in FyneApp.toml
APP_DISPLAY := Deye Monitor
GUI_ARTIFACT_darwin  := $(APP_DISPLAY).app
GUI_ARTIFACT_linux   := $(APP_DISPLAY).tar.xz
GUI_ARTIFACT_windows := $(APP_DISPLAY).exe
APK_NAME := Deye_Monitor.apk

GO   := go
FYNE := fyne

HOST_OS   := $(shell $(GO) env GOOS)
HOST_ARCH := $(shell $(GO) env GOARCH)

.DEFAULT_GOAL := help

# --- Meta --------------------------------------------------------------------
.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: all
all: tui gui gui-android ## Build TUI + host GUI + Android APK -> bin/

# --- TUI (host OS only) ------------------------------------------------------
.PHONY: tui
tui: ## Build the TUI for the host OS -> bin/deye-monitor
	@mkdir -p $(BIN)
	$(GO) build -o $(BIN)/$(APP_NAME) $(TUI_PKG)
	@echo "built $(BIN)/$(APP_NAME) ($(HOST_OS)/$(HOST_ARCH))"

# --- GUI: native packaging (host OS must match target) -----------------------
.PHONY: gui
gui: ## Package the GUI for the host OS -> bin/
	@mkdir -p $(BIN)
	cd $(GUI_DIR) && $(FYNE) package
	mv "$(GUI_DIR)/$(GUI_ARTIFACT_$(HOST_OS))" "$(BIN)/"
	@echo "built $(BIN)/$(GUI_ARTIFACT_$(HOST_OS))"

.PHONY: gui-darwin
gui-darwin: ## Package the GUI as a macOS .app -> bin/
	@mkdir -p $(BIN)
	cd $(GUI_DIR) && $(FYNE) package -os darwin
	mv "$(GUI_DIR)/$(GUI_ARTIFACT_darwin)" "$(BIN)/"
	@echo "built $(BIN)/$(GUI_ARTIFACT_darwin)"

.PHONY: gui-linux
gui-linux: ## Package the GUI for Linux -> bin/
	@mkdir -p $(BIN)
	cd $(GUI_DIR) && $(FYNE) package -os linux
	mv "$(GUI_DIR)/$(GUI_ARTIFACT_linux)" "$(BIN)/"
	@echo "built $(BIN)/$(GUI_ARTIFACT_linux)"

.PHONY: gui-windows
gui-windows: ## Package the GUI for Windows -> bin/
	@mkdir -p $(BIN)
	cd $(GUI_DIR) && $(FYNE) package -os windows
	mv "$(GUI_DIR)/$(GUI_ARTIFACT_windows)" "$(BIN)/"
	@echo "built $(BIN)/$(GUI_ARTIFACT_windows)"

.PHONY: gui-ios
gui-ios: ## Package the GUI for iOS (needs Xcode, run on macOS)
	cd $(GUI_DIR) && $(FYNE) package -os ios

.PHONY: gui-android
gui-android: ## Build a sideload-ready Android APK -> bin/Deye_Monitor.apk
	@mkdir -p $(BIN)
	$(GUI_DIR)/build-android.sh
	mv "$(GUI_DIR)/$(APK_NAME)" "$(BIN)/$(APK_NAME)"
	@echo "built $(BIN)/$(APK_NAME)"

# --- GUI: desktop cross-build from any host (needs fyne-cross + Docker) -------
.PHONY: gui-cross-linux
gui-cross-linux: ## Cross-build the Linux GUI (fyne-cross + Docker)
	fyne-cross linux -arch=amd64,arm64 -app-id $(APP_ID) -icon $(GUI_DIR)/$(ICON) $(GUI_PKG)

.PHONY: gui-cross-windows
gui-cross-windows: ## Cross-build the Windows GUI (fyne-cross + Docker)
	fyne-cross windows -arch=amd64 -app-id $(APP_ID) -icon $(GUI_DIR)/$(ICON) $(GUI_PKG)

# --- Quality -----------------------------------------------------------------
.PHONY: check
check: vet test ## Run go vet + tests

.PHONY: test
test: ## Run all tests
	$(GO) test ./...

.PHONY: race
race: ## Run tests with the race detector
	$(GO) test -race ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format all Go code
	gofmt -w .

# --- Tooling / cleanup -------------------------------------------------------
.PHONY: tools
tools: ## Install the Fyne packaging CLIs (fyne + fyne-cross)
	$(GO) install fyne.io/tools/cmd/fyne@latest
	$(GO) install github.com/fyne-io/fyne-cross@latest

.PHONY: clean
clean: ## Remove build artifacts (keeps the signing keystore)
	rm -rf $(BIN) fyne-cross
	rm -rf $(GUI_DIR)/*.app
	rm -f $(GUI_DIR)/*.apk $(GUI_DIR)/*.apk.idsig
	rm -f $(GUI_DIR)/*.tar.xz $(GUI_DIR)/*.tar.gz $(GUI_DIR)/*.exe $(GUI_DIR)/*.zip
