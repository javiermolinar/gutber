# gutberg

A no distraction terminal UI reader for Project Gutenberg books.
<img width="1024" height="1024" alt="johannes-gutenberg-press-2" src="https://github.com/user-attachments/assets/0ea18785-992b-419a-87ae-2ca11d53fb66" />


## Features
- Search authors by prefix
- Browse and read downloaded books
- Chapter navigation and page tracking
- Adjustable text size

## Build (Go required)

```bash
go build
./gutberg
```

## Usage
```bash
./gutberg
```

Controls:
- Author search: type to filter, Enter to search books
- Library: Enter open, s search, c chapters, b back
- Reader: Enter/Space/pgdown next, pgup/back prev, +/- size, c chapters, b library, s search, q quit

<img width="1274" height="638" alt="Screenshot 2026-01-17 at 16 11 37" src="https://github.com/user-attachments/assets/14988302-3784-42be-b2cd-5ac7adc5afce" />


<img width="1253" height="645" alt="Screenshot 2026-01-17 at 16 08 57" src="https://github.com/user-attachments/assets/ba143ea0-56ff-4b55-9502-6f13b3114d7b" />

<img width="1251" height="652" alt="Screenshot 2026-01-17 at 16 09 09" src="https://github.com/user-attachments/assets/da52c238-a017-402c-8903-d952c8d8f334" />

<img width="1271" height="651" alt="Screenshot 2026-01-17 at 16 09 29" src="https://github.com/user-attachments/assets/2fa26233-6ab3-4ef0-a388-e39fe56e7b7e" />


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
