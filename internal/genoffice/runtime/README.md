This directory contains generated GenOffice bundles, fonts, PDF assets and notices.
They are ignored by Git and regenerated from `office/src`, `office/vendor` and
`office/package-lock.json` by `npm --prefix frontend run prebuild`.
Wails runs this preparation automatically before building the frontend.

`xlsx-sidecar.exe` is a required vendored engine rather than a generated JS asset;
it remains tracked and must be included in the embedded runtime.
