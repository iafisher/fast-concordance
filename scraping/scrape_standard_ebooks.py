import argparse
import contextlib
import pathlib
import shutil
import subprocess
import tempfile
from html.parser import HTMLParser
from typing import List


def main(repo: str) -> None:
    destdir = pathlib.Path("examples") / repo

    with contextlib.suppress(FileNotFoundError):
        shutil.rmtree(destdir)

    with tempfile.TemporaryDirectory() as tmpdir:
        subprocess.run(
            [
                "git",
                "clone",
                "--depth",
                "1",
                f"https://github.com/standardebooks/{repo}.git",
                tmpdir,
            ]
        )
        tmpdirpath = pathlib.Path(tmpdir)
        for html_file in tmpdirpath.glob("src/epub/text/*.xhtml"):
            html_text = html_file.read_text()
            plaintext = html_to_txt(html_text)
            destdir.mkdir(exist_ok=True)
            outpath = destdir / (html_file.stem + ".txt")
            outpath.write_text(plaintext)
            print(f"wrote {outpath}")


class TextExtractor(HTMLParser):
    buffer: List[str]

    def __init__(self) -> None:
        super().__init__()
        self.buffer = []

    def handle_data(self, data: str) -> None:
        self.buffer.append(data)

    def finish(self) -> str:
        return " ".join(self.buffer)


def html_to_txt(html_text: str) -> str:
    extractor = TextExtractor()
    extractor.feed(html_text)
    return extractor.finish()


if __name__ == "__main__":
    argparser = argparse.ArgumentParser()
    argparser.add_argument("repo")
    args = argparser.parse_args()

    main(args.repo)
