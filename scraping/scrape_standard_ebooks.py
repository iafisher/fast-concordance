import argparse
import json
import os
import pathlib
import re
import shutil
import statistics
import subprocess
import tempfile
import time
import xml.etree.ElementTree as ET
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
        if repo in (
            "manual",
            "standard-blackletter",
            "sublime-text-se-plugin",
            "tools",
            "web",
        ):
            continue

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


def capitalize_author(author: str) -> str:
    words = author.split()
    for i in range(len(words)):
        word = words[i]

        if word not in ("de",):
            words[i] = word.capitalize()
            continue

    return " ".join(words)


def capitalize_title(title: str) -> str:
    words = title.split()
    for i in range(len(words)):
        word = words[i]
        if all(c.lower() == "i" for c in word):
            # e.g., "III" in "Richard III"
            words[i] = word.upper()
            continue

        if i == 0 or word not in ("and", "of", "the"):
            words[i] = word.capitalize()
            continue

    return " ".join(words)


def parse_dir_name(name: str) -> dict:
    parts = name.split("_")
    assert len(parts) >= 2, parts
    author = capitalize_author(parts[0].replace("-", " "))
    title = capitalize_title(parts[1].replace("-", " "))
    print(author, title)
    return dict(author=author, title=title, url="")


def find_or_blank(root, xpath: str) -> str:
    node = root.find(xpath)
    return node.text if node is not None else ""


def find_multiple_authors(root) -> List[str]:
    r = []
    i = 1
    while True:
        text = find_or_blank(root, f".//*[@id='author-{i}']")
        if not text:
            break

        r.append(text)
        i += 1

    return r


def get_manifest_entry_from_dir(subpath: pathlib.Path) -> dict:
    opf_path = subpath / "content.opf"
    if not opf_path.exists():
        print(f"==> {opf_path} does not exist, falling back to filename parsing")
        return parse_dir_name(subpath.name)

    tree = ET.parse(opf_path)
    root = tree.getroot()
    title = find_or_blank(root, ".//*[@id='title']")
    author = find_or_blank(root, ".//*[@id='author']")
    uid = find_or_blank(root, ".//*[@id='uid']")
    if uid.startswith("url:"):
        url = uid[4:]
    else:
        url = ""

    if not title:
        print(f"==> title missing from {opf_path}, falling back to filename parsing")
        return parse_dir_name(subpath.name)

    if not author:
        authors = find_multiple_authors(root)
        if not authors:
            print(
                f"==> author missing from {opf_path}, falling back to filename parsing"
            )
            return parse_dir_name(subpath.name)
        else:
            author = " & ".join(authors)

    return dict(title=title, author=author, url=url)


def extract_text(dir: str, *, outdir: str, force: bool, manifest_only: bool) -> None:
    start_time = time.time()
    outdir = pathlib.Path(outdir)

    nchars = 0
    manifest = {}
    for subpath in pathlib.Path(dir).iterdir():
        if not subpath.is_dir():
            continue

        out_subdir = outdir / subpath.name
        out_subdir.mkdir(exist_ok=True)

        manifest[subpath.name] = get_manifest_entry_from_dir(subpath)
        if manifest_only:
            continue

        plaintext = []
        for xhtml_path in subpath.glob("*.xhtml"):
            if xhtml_path.name in (
                "colophon.xhtml",
                "dedication.xhtml",
                "dramatis-personae.xhtml",
                "endnotes.xhtml",
                "frontispiece.xhtml",
                "glossary.xhtml",
                "halftitlepage.xhtml",
                "imprint.xhtml",
                "loi.xhtml",  # List of Illustrations
                "titlepage.xhtml",
                "translator-note.xhtml",
                "translators-note.xhtml",
                "translators-dedication.xhtml",
                "translators-preface.xhtml",
                "uncopyright.xhtml",
            ) or xhtml_path.name.startswith(
                ("appendix", "epipgraph", "translators-intro-")
            ):
                continue

            html_text = xhtml_path.read_text()
            plaintext.append(html_to_txt(html_text))

        out_path = out_subdir / "merged.txt"

        text = "\n\n".join(plaintext)
        out_path.write_text(text)
        print(f"==> wrote: {out_path}")
        nchars += len(text)

    (outdir / "manifest.json").write_text(json.dumps(manifest))

    if not manifest_only:
        duration_secs = time.time() - start_time
        print(f"==> wrote {nchars} characters in {duration_secs:.1f} secs")


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
            outpath_xhtml = destdir / html_file.name
            shutil.copyfile(html_file, outpath_xhtml)
            print(f"==> wrote: {outpath_xhtml}")

        opf_path = tmpdirpath / "src" / "epub" / "content.opf"
        if opf_path.exists():
            outpath_opf = destdir / "content.opf"
            shutil.copyfile(opf_path, outpath_opf)
            print(f"==> wrote: {outpath_opf}")

    return True


whitespace_pattern = re.compile(r"\s+")


class TextExtractor(HTMLParser):
    buffer: List[str]
    tags_to_ignore = set(["head", "h1", "h2", "h3", "h4", "h5", "h6", "hgroup"])

    def __init__(self) -> None:
        super().__init__()
        self.buffer = []
        self.ignore_stack = []

    def handle_starttag(self, tag, attrs) -> None:
        if tag in self.tags_to_ignore:
            self.ignore_stack.append(tag)

    def handle_endtag(self, tag):
        if self.ignore_stack and self.ignore_stack[-1] == tag:
            self.ignore_stack.pop()

    def handle_data(self, data: str) -> None:
        if len(self.ignore_stack) > 0:
            return

        self.buffer.append(data)

    def finish(self) -> str:
        return whitespace_pattern.sub(" ", " ".join(self.buffer))


def html_to_txt(html_text: str) -> str:
    extractor = TextExtractor()
    extractor.feed(html_text)
    return extractor.finish()


if __name__ == "__main__":
    # TODO: split this into proper subcommands and merge in scrape_repo_list.py
    argparser = argparse.ArgumentParser()
    group = argparser.add_mutually_exclusive_group(required=True)
    group.add_argument(
        "--from-file", metavar="FILE", help="file of repos to scrape (one per line)"
    )
    group.add_argument("--repo", help="repo to scrape")
    group.add_argument(
        "--extract-only",
        action="store_true",
        help="extract text from HTML only, don't download",
    )

    argparser.add_argument("-d", "--dir", help="destination directory", required=True)
    argparser.add_argument(
        "-f", "--force", action="store_true", help="replace existing downloads"
    )
    argparser.add_argument(
        "--manifest-only",
        action="store_true",
        help="with --extract-only, only generate the manifest.json file",
    )
    argparser.add_argument(
        "--limit",
        metavar="N",
        type=int,
        default=-1,
        help="limit number of repos to scrape",
    )
    argparser.add_argument("--outdir", help="out directory for --extract-only")
    args = argparser.parse_args()

    if args.from_file:
        download_from_list(
            args.from_file, dir=args.dir, force=args.force, limit=args.limit
        )
    elif args.extract_only:
        extract_text(
            dir=args.dir,
            outdir=args.outdir,
            force=args.force,
            manifest_only=args.manifest_only,
        )
    else:
        download_one(args.repo, dir=args.dir, force=args.force)
