# Bazzite ujust Picker

A TUI application to browse and run ujust recipes in Bazzite.

<img width="525" alt="image" src="https://github.com/user-attachments/assets/cd0d3d3b-46d2-4e95-90a3-1f94a7496d45" />


## Installation

### From Binary Release

Download the latest release from the [releases page](https://github.com/bazzite-org/bazzite-ujust-picker/releases).

```bash
# Extract and install to /usr/local/bin
sudo tar -xzf ujust-picker_Linux_x86_64.tar.gz -C /usr/local/bin ujust-picker
```

### From Source

```bash
# Clone the repository
git clone https://github.com/bazzite-org/bazzite-ujust-picker.git
cd bazzite-ujust-picker

# Build and install
go build -o ujust-picker
sudo cp ujust-picker /usr/local/bin/
```

## Usage

Simply run the picker and navigate using keyboard controls:

```bash
ujust-picker
```

### Controls

- **←/→**: Navigate between categories
- **↑/↓**: Navigate recipes
- **Enter**: Select and run recipe
- **Esc/q/Ctrl+C**: Exit

## Development

### Requirements

- Go 1.24 or later

### Setup Development Environment

```bash
# Clone the repository
git clone https://github.com/bazzite-org/bazzite-ujust-picker.git
cd bazzite-ujust-picker

# Install dependencies
go mod tidy

# Run in development mode
go run .
```

### Building

```bash
go build -o ujust-picker
```

### CI/CD Pipeline

This project uses GitHub Actions for continuous integration and delivery:

- Pushing to the `main` branch automatically creates a new tag and triggers a release
- Tags are created using the current date (YYYY-MM-DD) format
- Multiple releases on the same day will have an incrementing counter (e.g., 2025-05-31.1, 2025-05-31.2)
- Binary builds are created for Linux (x86_64 and arm64)
- Releases are automatically published to GitHub Releases

The workflow configuration is in `.github/workflows/release.yml` and uses GoReleaser for building and packaging.

## License

MIT
