import argparse
import os
import pathlib
import shutil
import statistics
import subprocess
import tempfile
import time
from html.parser import HTMLParser
from typing import List


SLEEP_SECS = 1.5


def download_from_list(list_file: str, dir: str, *, force: bool, limit: int) -> None:
    with open(list_file, "r") as f:
        # scrape_repo_list.py includes 'standardebooks/' at the beginning which we need
        # to strip
        repos = [line.strip().split("/")[-1] for line in f.readlines()]

    times = []
    num_to_download = limit if limit != -1 else len(repos)
    last_skipped = True  # so we don't sleep the first time
    for i, repo in enumerate(repos, start=1):
        if not last_skipped:
            print(f"==> sleeping {SLEEP_SECS}s")
            time.sleep(SLEEP_SECS)

        start_time = time.time()
        last_skipped = not download_one(repo, dir, force=force)
        if last_skipped:
            continue

        time_elapsed = time.time() - start_time
        perc = i / num_to_download
        print(
            f"==> finished {i} of {num_to_download} ({perc:.1%}) in {time_elapsed:.1f}s"
        )

        if num_to_download == i:
            break

        times.append(time_elapsed)
        mean_time = statistics.mean(times)
        time_remaining = (num_to_download - i) * (mean_time + SLEEP_SECS)
        mins_remaining, secs_remaining = divmod(time_remaining, 60)
        hours_remaining, mins_remaining = divmod(mins_remaining, 60)
        if hours_remaining > 0:
            print(
                f"==> estimated time remaining: {hours_remaining:.0f}h {mins_remaining:.0f}m {secs_remaining:.0f}s"
            )
        else:
            print(
                f"==> estimated time remaining: {mins_remaining:.0f}m {secs_remaining:.0f}s"
            )


def extract_text(dir: str, *, force: bool) -> None:
    for xhtml_path in pathlib.Path(dir).glob("**/*.xhtml"):
        txt_path = xhtml_path.parent / (xhtml_path.stem + ".txt")
        if txt_path.exists():
            if force:
                print(f"==> overwriting: {txt_path}")
            else:
                print(f"==> skipping: {txt_path}")
                continue

        html_text = xhtml_path.read_text()
        plaintext = html_to_txt(html_text)
        txt_path.write_text(plaintext)
        print(f"==> wrote: {txt_path}")


def download_one(repo: str, dir: str, *, force: bool) -> bool:
    """
    Returns true if downloaded (as opposed to skipped).
    """
    os.makedirs(dir, exist_ok=True)

    destdir = pathlib.Path(dir) / repo
    if os.path.exists(destdir):
        if force:
            print(f"==> removing existing directory: {destdir}")
            shutil.rmtree(destdir)
        else:
            print(f"==> skipping existing directory: {destdir}")
            return False
    else:
        os.makedirs(destdir)

    with tempfile.TemporaryDirectory() as tmpdir:
        url = f"https://github.com/standardebooks/{repo}.git"
        print(f"==> scraping {url}")

        subprocess.run(["git", "clone", "--depth", "1", url, tmpdir], check=True)
        tmpdirpath = pathlib.Path(tmpdir)
        for html_file in tmpdirpath.glob("src/epub/text/*.xhtml"):
            html_text = html_file.read_text()
            outpath_xhtml = destdir / html_file.name
            outpath_xhtml.write_text(html_text)
            print(f"==> wrote: {outpath_xhtml}")

            plaintext = html_to_txt(html_text)
            outpath_txt = destdir / (html_file.stem + ".txt")
            outpath_txt.write_text(plaintext)
            print(f"==> wrote: {outpath_txt}")

    return True


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
    group = argparser.add_mutually_exclusive_group(required=True)
    group.add_argument(
        "--from-file", metavar="FILE", help="file of repos to scrape (one per line)"
    )
    group.add_argument("--repo", help="repo to scrape")
    group.add_argument(
        "--extract-only", help="extract text from HTML only, don't download"
    )

    argparser.add_argument("-d", "--dir", help="destination directory", required=True)
    argparser.add_argument(
        "-f", "--force", action="store_true", help="replace existing downloads"
    )
    argparser.add_argument(
        "--limit",
        metavar="N",
        type=int,
        default=-1,
        help="limit number of repos to scrape",
    )
    args = argparser.parse_args()

    if args.from_file:
        download_from_list(
            args.from_file, dir=args.dir, force=args.force, limit=args.limit
        )
    elif args.extract_only:
        extract_text(dir=args.dir, force=args.force)
    else:
        download_one(args.repo, dir=args.dir, force=args.force)
