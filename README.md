# gutberg

Terminal UI reader for Project Gutenberg books.

## Features
- Search authors by prefix
- Browse and read downloaded books
- Chapter navigation and page tracking
- Adjustable text size

## Usage
Build and run:

```bash
go build
./gutberg
```

Controls:
- Author search: type to filter, Enter to search books
- Library: Enter open, s search, c chapters, b back
- Reader: Enter/Space/pgdown next, pgup/back prev, +/- size, c chapters, b library, s search, q quit

## Config
A config file is created at `~/.config/gutberg/gutberg.toml` with:

```toml
books_dir = "~/.config/gutberg/books"
state_file = "~/.config/gutberg/state.json"
```

Downloaded books are stored in `books_dir` and reading progress is stored in `state_file`.

## Build Matrix
GitHub Actions builds binaries for:
- Linux amd64/arm64
- macOS amd64/arm64
- Windows amd64
