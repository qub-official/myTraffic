# Publishing myTraffic on GitHub (drag and drop)

Your whole project folder is the repository. You'll do two things:

1. Upload the code + website so the repo and the live site exist.
2. Attach the built installer to a **Release** so the Download buttons work.

---

## What to upload, and what to leave out

From `c:\Users\KELLYBLUE\Desktop\projects\myTraffic`, upload these:

```
index.html          app/            build.ps1
assets/             installer/      README.md
.nojekyll           PUBLISH_GUIDE.md  LICENSE
.gitignore          ROADMAP.md
```

**Leave these out** (they are build output; the installer goes to Releases):

- `app\mytraffic.exe`
- `installer\Output\` (the `myTraffic-Setup-1.0.0.exe` — this goes to a Release in step 3)

> Tip: `.nojekyll` and `.gitignore` are hidden/dot files. In File Explorer turn on
> **View → Show → Hidden items** so you can select them.

---

## 1. Create the repository

1. Sign in at https://github.com as **qub-official**.
2. Click **+** (top right) → **New repository**.
3. Name: `myTraffic`
4. Description: `Free, open-source, local-first web traffic workbench.`
5. Choose **Public**.
6. Leave **Add a README**, **.gitignore** and **license** **unchecked** — they already exist.
7. Click **Create repository**.

## 2. Upload the files

1. On the new empty repo page, click the **uploading an existing file** link.
2. In File Explorer open the `myTraffic` folder. Select the items listed above.
   - To preserve the folders, it's easiest to drag `app`, `assets` and
     `installer` (without its `Output` subfolder) in one go along with the
     loose files. GitHub keeps the folder structure.
   - `index.html` must land at the **top level** of the repo (not inside a subfolder),
     or the website won't load.
3. Drop them onto the page and wait until every file is listed.
4. Commit message: `Initial commit — myTraffic`. Keep **Commit directly to the main branch**.
5. Click **Commit changes**.
6. Confirm the repo shows `index.html`, `assets/`, `app/`, `installer/` (with the
   `.iss` script but no `Output`), `README.md` and `LICENSE`.

## 3. Turn on the website (GitHub Pages)

1. Repo → **Settings** → **Pages**.
2. **Source**: **Deploy from a branch**.
3. Branch **main**, folder **/ (root)** → **Save**.
4. Wait 1–3 minutes, refresh. Your site is live at:
   **https://qub-official.github.io/myTraffic/**
5. Optional: on the repo home page click the gear by **About** and tick
   **Use your GitHub Pages website**.

## 4. Publish the installer as a Release

The site's **Download** buttons link to `releases/latest`, so add a release:

1. Repo → **Releases** (right sidebar) → **Create a new release**.
2. **Choose a tag** → type `v1.0.0` → **Create new tag on publish**.
3. Release title: `myTraffic 1.0.0`.
4. Notes: a line or two (e.g. "First public release. Windows x64.").
5. Under **Attach binaries**, drag in:
   `installer\Output\myTraffic-Setup-1.0.0.exe`
6. Click **Publish release**.

The Download buttons on the site now work.

---

## Updating later

- **Website or code:** open the file on GitHub → pencil to edit, or
  **Add file → Upload files** and drop the new version with the same name.
  Pages redeploys in a minute or two.
- **New app version:** rebuild (`build.ps1`), then create a new Release with a
  new tag (`v1.0.1`) and attach the new installer.

## Troubleshooting

- **Site 404:** `index.html` must be at the repo root, and Pages set to `main` / root.
- **Site unstyled:** make sure `assets/styles.css` uploaded inside an `assets/`
  folder — names are case-sensitive.
- **Download 404:** you haven't published a Release with the `.exe` attached yet (step 4).
- **Old version shows:** hard refresh with Ctrl+F5.
