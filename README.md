# Bible Reader

A web-based Bible reading application built with Go and JavaScript, using SWORD/Crosswire zText format files.

## Features

- Browse all books of the Bible (Old and New Testament)
- Read chapters with verse-by-verse display
- Navigate between chapters using Previous/Next buttons or keyboard arrows
- Jump to any chapter using the dropdown selector
- Responsive design with Tailwind CSS
- Clean, distraction-free reading interface

## Prerequisites

- Go 1.25.4 or higher

## API Endpoints

- `GET /api/books` - Returns list of all books with chapter counts
- `GET /api/chapter?book={name}&chapter={number}` - Returns verses for a specific chapter

## Keyboard Shortcuts

- `←` (Left Arrow) - Previous chapter
- `→` (Right Arrow) - Next chapter

### Frontend styles

Run `npm ci && npm run build:css` after changing HTML or JavaScript classes.
Commit `static/tailwind.css` alongside those changes; Go embeds it in the binary.
Tailwind 3 preserves the existing CDN styling without a major-version migration.

For an alternate local port, set `LISTEN_ADDR=127.0.0.1:8081` when starting the server.
