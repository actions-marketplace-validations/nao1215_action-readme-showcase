# Security Policy

## Reporting a Vulnerability

Please report security issues by email to [n.chika156@gmail.com](mailto:n.chika156@gmail.com), not in a public issue. I will reply and work on a fix as soon as I can.

## Supported Versions

Only the latest release gets security fixes. Reference the action at `@v0` to pick them up, or pin a release and let Dependabot update it.

## Notes on the token

The star history graph needs a personal access token. It only reads public data, so create a fine-grained token with "Public repositories" access and no permissions. The action sends the token only to the GitHub API (`GITHUB_API_URL` and `GITHUB_GRAPHQL_URL`) and never writes it to a file or to the log.

The binary the action runs is downloaded from this repository's release and checked against the release's `checksums.txt`. Each release also carries a build provenance attestation, which you can check with `gh attestation verify <archive> --repo nao1215/action-readme-showcase`.
