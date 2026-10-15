---
name: ffc-ppt-live
description: Generate or edit native PowerPoint PPTX inside ffc using its GenOffice engine and live canvas. Use for slide decks and follow-up slide edits in this application's workspace.
---

Use the application's office MCP tools when available. The equivalent CLI is
`node ABSOLUTE_SKILL_DIR/scripts/office.cjs ACTION --path relative/deck.pptx`.
The application supplies the private endpoint and capability in the CLI environment.
Never display or store FFC_OFFICE_TOKEN or FFC_OFFICE_ENDPOINT.

1. Plan an outline and coherent visual style, then `open --create` before generation so the preview appears immediately.
2. Generate each page with `page --input page.json --revision N --request-id UNIQUE_ID`. Each completed page is inserted immediately; do not wait to build the entire presentation. The first page replaces the initial empty page. Read the response's revision before the next operation.
3. To modify a deck, `read` first. It returns 0-based slide indexes and durable element IDs. User page 3 means index 2. Read `guide --group text|element|slide|insert|table|deck` for exact operation schemas; send native ops with `apply --input ops.json --revision N --request-id UNIQUE_ID`.
4. Run `audit`, resolve actionable layout findings, then finish with `save --revision N` and verify savedRevision equals revision. Return the real workspace path. The final PPTX must contain editable text/shapes; images can be placed as assets, but do not flatten complete pages into screenshots. If external edits prevent saving, use `save --copy output/recovered.pptx --revision N` to preserve a separate copy; never overwrite the newer external file.

Page spec uses a 1280×720 pixel design canvas. Colors are #RRGGBB.
Native generation example:

```json
{
  "background": "#F4F7FC",
  "elements": [
    { "type": "shape", "shape": "rect", "x": 0, "y": 0, "w": 1280, "h": 16, "fill": "#286BCD" },
    { "type": "text", "x": 72, "y": 96, "w": 1120, "h": 120,
      "paragraphs": [{ "runs": [{ "text": "Presentation title", "sizePt": 36, "bold": true, "font": "Microsoft YaHei", "color": "#203650" }] }] }
  ]
}
```

Image elements use `{type:"image", url:"assets/photo.png", x, y, w, h}`.
Use workspace-local images; obtain requested online assets separately.
The page builder supports shapes, text and images. Tables/charts use native operations
from the insert/table guides rather than inventing unsupported page-spec types.

CLI examples (replace the wrapper path and revision with actual values):

```text
node WRAPPER open --path output/deck.pptx --create
node WRAPPER page --path output/deck.pptx --input first-page.json --revision 0 --request-id page-1
node WRAPPER read --path output/deck.pptx
node WRAPPER guide --path output/deck.pptx --group text
node WRAPPER apply --path output/deck.pptx --input edits.json --revision 1 --request-id edit-1
node WRAPPER save --path output/deck.pptx --revision 2
```

`apply` input is an operation array (or an object with an `ops` array).
For example, `setText` uses `paragraphs:[{srcPara:0,runs:[{text:"New title",srcPara:0,srcRun:0}]}]`
to retain original formatting. Operation geometry is EMU, 1 px = 9525 EMU;
font sizes use the guide's point fields. Prefer durable IDs when slides can move.

Every mutating request requires the current revision. A version conflict means re-read
and target the updated deck; do not force-write the file. Reuse the same request ID
and identical payload for a retry so generation cannot duplicate pages. Atomic invalid
batches change nothing. A saved-file conflict or file lock must be reported; retain
the open document and offer retry or another output rather than claiming success.

The application's canvas is authoritative for live edits. Do not launch an external
Office application to preview, call the old OfficeCLI wrapper, or invoke GenOffice's
separate cloud AI. Complex imported Office features and animation playback require
separate verification; static preview does not establish PowerPoint/WPS visual parity.
