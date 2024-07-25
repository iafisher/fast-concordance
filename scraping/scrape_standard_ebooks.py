import argparse
import glob
import os
import pathlib
import shutil
import subprocess
import tempfile
from html.parser import HTMLParser
from typing import List


def main(repo: str) -> None:
    destdir = f"examples/{repo}"
    shutil.rmtree(destdir, ignore_errors=True)

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
        for html_file in glob.glob(f"{tmpdir}/src/epub/text/*.xhtml"):
            print(html_file)
            html_text = pathlib.Path(html_file).read_text()
            plaintext = html_to_txt(html_text)
            os.makedirs(destdir, exist_ok=True)
            pathlib.Path(
                destdir
                + "/"
                + os.path.splitext(os.path.basename(html_file))[0]
                + ".txt"
            ).write_text(plaintext)


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
