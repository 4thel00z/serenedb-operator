"""Writes the operator logo (the Watcher mark) and its wordmark lockup into assets/."""
from __future__ import annotations

import pathlib

VIOLET = "#650AFF"
SKY = "#80BFFF"
INK = "#1B0B3F"
PALE = "#EDE7FF"
WHITE = "#FFFFFF"
FONT = "ui-sans-serif, -apple-system, 'Helvetica Neue', Arial, sans-serif"

ASSETS = pathlib.Path(__file__).resolve().parent.parent / "assets"


def watcher(ink: str) -> str:
    return (
        f'<path d="M 4 50 C 22 18, 78 18, 96 50 C 78 82, 22 82, 4 50 Z" fill="{ink}"/>'
        f'<circle cx="50" cy="50" r="24" fill="{VIOLET}"/>'
        '<g transform="translate(50 50) scale(0.6) rotate(45)">'
        f'<ellipse rx="25" ry="21.4" fill="{SKY}"/>'
        f'<ellipse cx="8.2" cy="-8.2" rx="12.5" ry="10.5" fill="{WHITE}"/>'
        "</g>"
    )


def mark(ink: str) -> str:
    return (
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="256" height="256" role="img" '
        'aria-label="SereneDB Operator">'
        f"{watcher(ink)}</svg>\n"
    )


def lockup(ink: str, word: str) -> str:
    return (
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 560 100" width="560" height="100" role="img" '
        'aria-label="SereneDB Operator">'
        f"{watcher(ink)}"
        f'<text x="118" y="65" font-family="{FONT}" font-size="46" font-weight="800" letter-spacing="-1.5" '
        f'fill="{word}">SereneDB</text>'
        f'<text x="336" y="65" font-family="{FONT}" font-size="46" font-weight="400" letter-spacing="-1" '
        f'fill="{VIOLET}">Operator</text>'
        "</svg>\n"
    )


def main() -> None:
    ASSETS.mkdir(exist_ok=True)
    files = {
        "logo.svg": mark(INK),
        "logo-dark.svg": mark(PALE),
        "logo-lockup.svg": lockup(INK, INK),
        "logo-lockup-dark.svg": lockup(PALE, PALE),
    }
    for name, content in files.items():
        (ASSETS / name).write_text(content)
        print(ASSETS / name)


if __name__ == "__main__":
    main()
