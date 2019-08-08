[![Build Status](https://img.shields.io/travis/function61/varasto.svg?style=for-the-badge)](https://travis-ci.org/function61/varasto)
[![Download](https://img.shields.io/badge/Download-bintray%20latest-blue.svg?style=for-the-badge)](https://bintray.com/function61/dl/varasto/_latestVersion#files)
[![GoDoc](https://img.shields.io/badge/godoc-reference-5272B4.svg?style=for-the-badge)](https://godoc.org/github.com/function61/varasto)

Software defined distributed storage array with custom replication policies and strong
emphasis on integrity and encryption.

See [screenshots](docs/screenshots.md) to get a better picture.

Status
------

Currently *under heavy development*. Works so robustly (blobs currently cannot be
deleted so if metadata DB is properly backed up, you can't lose data) that I'm already
moving *all my files in*, but I wouldn't yet recommend this for anybody else. Also proper
access controls are nowhere to be found.


Features
--------

| Status | Feature                     | Details                               |
|--------|-----------------------------|---------------------------------------|
| ✓      | Supported OSs               | Linux, Windows (Mac might come later) |
| ✓      | Supported architectures     | amd64, ARM (= PC, Raspberry Pi etc.) |
| ✓      | Supported storage methods   | Local disks or cloud services (AWS S3, Google Drive) |
| ✓      | [Integrated backups](docs/guide_setting-up-backup.md) | Use optional built-in backup to automatically upload encrypted backup of your metadata DB to AWS S3. If you don't like it, there's interface for external backup tools as well. |
| ✓      | Compression                 | Storing well-compressible files? They'll be compressed automatically (if it compresses well) & transparently |
| ✓      | Metadata support            | Can use metadata sources for automatically fetching movie/TV series info, poster images etc. |
| ✓      | All files in one place      | Never again forget on which disk a particular file was stored - it's all in one place even if you have 100 disks! |
| ✓      | Thumbnails for photos       | Automatic thumbnailing of photos/pictures |
| TODO   | Thumbnails for videos       | Automatic thumbnailing of videos |
| TODO   | Video & audio transcoding   | Got movie in 4K resolution but your FullHD resolution phone doesn't have the power or bandwidth to watch it? |
| ✓      | Data access methods         | 1) Clone collection to your computer 2) Open/stream files from web UI 3) Access files via network share 4) Access via Linux FUSE interface |
| TODO   | Atomic snapshots            | Uses LVM on Linux and shadow copies on Windows to grab consistent copies of files |
| ✓      | Data integrity              | Sha256 hashes verified on file write/read - detects bit rot immediately |
| ✓      | Data privacy                | All data is encrypted - each collection with a separate key so compromise of one collection does not compromise other data |
| ✓      | Data sensitivity            | You can mark different collections with different sensitivity levels and decide on login if you want to show only family-friendly content |
| ✓      | Data durability             | Transparently replicates your data to multiple disks / to offsite storage |
| ✓      | Per-collection durability   | To save money, we support storing important files with higher redundancy than less important files |
| ✓      | Transactional               | File or group of files are successfully committed or none at all. Practically no other filesystem does this |
| ✓      | Scheduled scrubbing         | Varasto can scan your disks periodically to detect failing disks ASAP |
| ✓   | [Ransomware protection](docs/guide_ransomware-protection.md) | Run Varasto on a separate security-hardened device/NAS to protect from ransomware, or configure replication to S3 ransomware-protected bucket |
| TODO   | Integrated SMART monitoring | Detect disk failures early |
| TODO   | Tiered storage              | Use SSD for super fast data ingestion, and transfer it in background to a spinning disk |
| TODO   | Multi-user                  | Have separate file hierarchies for your friends & family |
| TODO   | File sharing                | Share your own files to friends |
| TODO   | Offline drives              | We support use cases where you plug in a particular hard drive occasionally. Queued writes/deletes are applied when volume becomes available |


Docs
----

Design:

- [Terminology](docs/design_terminology.md)
- [Architecture / ideas & goals / inspired by / comparison to similar software](docs/design_architecture-ideas-goals-inspired-by-comparison-to-similar-software.md)

Operating:

- [How to install](docs/guide_how-to-install.md)
- [Setting up AWS S3](docs/guide_setting-up-s3.md)
- [Setting up Google Drive](docs/guide_setting-up-googledrive.md)
- [Setting up backup](docs/guide_setting-up-backup.md)
- [Setting up ransomware protection](docs/guide_ransomware-protection.md)

Developers:

- [Code documentation on GoDoc.org](https://godoc.org/github.com/function61/varasto)

Misc:

- [Security policy](https://github.com/function61/varasto/security/policy)


Philosophy
----------

- [How to Remember Your Life by Johnny Harris](https://www.youtube.com/watch?v=GLy4VKeYxD4)
