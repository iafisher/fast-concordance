import argparse
import json
import os
from pathlib import Path
from typing import Optional

import requests


def parse_link_header(header: str) -> Optional[str]:
    # link HTTP header format:
    #   link header: <https://api.github.com/user/10159941/repos?page=1>; rel="prev", <https://api.github.com/user/10159941/repos?page=3>; rel="next", <https://api.github.com/user/10159941/repos?page=35>; rel="last", <https://api.github.com/user/10159941/repos?page=1>; rel="first"

    parts = header.split(",")
    for part in parts:
        url, rel = part.split(";", maxsplit=1)
        if rel.strip() == 'rel="next"':
            return url.strip().lstrip("<").rstrip(">")

    return None


if __name__ == "__main__":
    argparse.ArgumentParser().parse_args()
    
    token = os.environ["GITHUB_TOKEN"]
    headers = {
        "Accept": "application/vnd.github+json",
        "Authorization": f"Bearer {token}",
        "X-GitHub-Api-Version": "2022-11-28",
    }

    next_url = "https://api.github.com/users/standardebooks/repos"
    counter = 1

    with open("payload_dump/repos.txt", "w") as f:
        while next_url is not None:
            print(f"fetching {counter}")
            response = requests.get(next_url, headers=headers)
            payload = response.json()

            output_path = Path("payload_dump") / f"response{counter}.json"
            output_path.write_text(json.dumps(payload))
            counter += 1

            for repo in payload:
                f.write(repo["full_name"] + "\n")
                print(repo["full_name"])

            link_header = response.headers["link"]
            print(f"link header: {link_header}")
            next_url = parse_link_header(link_header)
            print(f"next url: {next_url}")
