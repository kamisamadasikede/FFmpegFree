"""Independent ffc PDF helpers (MulanPSL-2.0); not derived from proprietary skills."""
import argparse
import json
import sys
from pathlib import Path


def reader(file, password=None):
    from pypdf import PdfReader
    path = Path(file)
    if not path.is_file() or path.stat().st_size > 100 * 1024 * 1024:
        raise ValueError("Input must be a regular PDF no larger than 100 MiB")
    result = PdfReader(path)
    if result.is_encrypted and (not password or not result.decrypt(password)):
        raise ValueError("Encrypted PDF: provide the correct --password")
    if len(result.pages) > 5000:
        raise ValueError("PDF exceeds the 5000 page limit")
    return result


def selected(spec, count):
    if not spec:
        return list(range(count))
    result = []
    for token in spec.split(","):
        bounds = token.strip().split("-")
        if len(bounds) > 2:
            raise ValueError("Use page ranges such as 1-3,5")
        start, end = int(bounds[0]), int(bounds[-1])
        if start < 1 or end < start or end > count:
            raise ValueError("Page range is outside the document")
        result.extend(range(start - 1, end))
    if not result or len(result) > 5000:
        raise ValueError("Invalid page selection")
    return result


def destination(file, force):
    path = Path(file)
    if path.exists() and not force:
        raise ValueError(f"Output already exists: {path}; use --force to replace it")
    path.parent.mkdir(parents=True, exist_ok=True)
    return path


def save_writer(writer, file, force):
    with destination(file, force).open("wb" if force else "xb") as stream:
        writer.write(stream)


def create_document(file, output, force):
    from html import escape
    from reportlab.lib import colors
    from reportlab.lib.pagesizes import A4
    from reportlab.lib.styles import getSampleStyleSheet
    from reportlab.pdfbase import pdfmetrics
    from reportlab.pdfbase.ttfonts import TTFont
    from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle, PageBreak
    data = json.loads(Path(file).read_text(encoding="utf-8-sig"))
    styles = getSampleStyleSheet()
    if data.get("font"):
        pdfmetrics.registerFont(TTFont("ffc-document", data["font"]))
        for style in styles.byName.values():
            style.fontName = "ffc-document"
    story = []
    if data.get("title"):
        story.extend([Paragraph(escape(str(data["title"])), styles["Title"]), Spacer(1, 14)])
    for block in data.get("blocks", []):
        kind = block.get("type", "paragraph")
        if kind == "pagebreak":
            story.append(PageBreak())
        elif kind == "table":
            rows = [[Paragraph(escape(str(value)), styles["Normal"]) for value in row] for row in block["rows"]]
            table = Table(rows, repeatRows=1, hAlign="LEFT")
            table.setStyle(TableStyle([("BACKGROUND", (0, 0), (-1, 0), colors.HexColor("#e6edf3")), ("GRID", (0, 0), (-1, -1), .4, colors.HexColor("#bdc8d2")), ("VALIGN", (0, 0), (-1, -1), "TOP"), ("LEFTPADDING", (0, 0), (-1, -1), 8), ("RIGHTPADDING", (0, 0), (-1, -1), 8), ("TOPPADDING", (0, 0), (-1, -1), 6), ("BOTTOMPADDING", (0, 0), (-1, -1), 6)]))
            story.extend([table, Spacer(1, 12)])
        elif kind in ("heading", "paragraph"):
            text = escape(str(block.get("text", ""))).replace("\n", "<br/>")
            story.extend([Paragraph(text, styles["Heading2" if kind == "heading" else "Normal"]), Spacer(1, 10)])
        else:
            raise ValueError(f"Unknown block type: {kind}")
    if not story:
        raise ValueError("Document has no content")
    dest = destination(output, force)
    # Writing through an exclusive stream protects an existing output even if it
    # appears after the initial path check.
    with dest.open("wb" if force else "xb") as stream:
        SimpleDocTemplate(stream, pagesize=A4, title=str(data.get("title", "")), author=str(data.get("author", "")), leftMargin=48, rightMargin=48, topMargin=48, bottomMargin=48).build(story)


def main(argv=None):
    parser = argparse.ArgumentParser(description="ffc PDF info/text/merge/split/render/create/fill")
    parser.add_argument("command", choices=["info", "text", "merge", "split", "render", "create", "fill"])
    parser.add_argument("inputs", nargs="+")
    parser.add_argument("--output")
    parser.add_argument("--pages")
    parser.add_argument("--values")
    parser.add_argument("--scale", type=float, default=1.25)
    parser.add_argument("--password")
    parser.add_argument("--force", action="store_true")
    args = parser.parse_args(argv)
    if args.command not in ("info", "text") and not args.output:
        parser.error("--output is required")
    if args.command == "create":
        create_document(args.inputs[0], args.output, args.force)
        return
    doc = reader(args.inputs[0], args.password)
    indices = selected(args.pages, len(doc.pages))
    if args.command == "info":
        fields = {name: {"type": str(value.get("/FT", "")), "value": str(value.get("/V", ""))} for name, value in (doc.get_fields() or {}).items()}
        print(json.dumps({"pages": len(doc.pages), "fields": fields, "metadata": {str(k): str(v) for k, v in (doc.metadata or {}).items()}}, ensure_ascii=False))
    elif args.command == "text":
        text = "\n\n".join(doc.pages[i].extract_text() or "" for i in indices)
        if args.output:
            dest = destination(args.output, args.force)
            with dest.open("w" if args.force else "x", encoding="utf-8") as out:
                out.write(text)
        else:
            print(text)
    elif args.command in ("merge", "split", "fill"):
        from pypdf import PdfWriter
        writer = PdfWriter()
        if args.command == "merge":
            readers = [doc] + [reader(file, args.password) for file in args.inputs[1:]]
            if sum(len(r.pages) for r in readers) > 5000:
                raise ValueError("Merged PDF exceeds 5000 pages")
            for part in readers:
                writer.append(part)
        elif args.command == "split":
            for i in indices:
                writer.add_page(doc.pages[i])
        else:
            if not args.values:
                parser.error("fill requires --values values.json")
            values = json.loads(Path(args.values).read_text(encoding="utf-8-sig"))
            unknown = set(values) - set(doc.get_fields() or {})
            if unknown:
                raise ValueError("Unknown PDF fields: " + ", ".join(sorted(unknown)))
            writer.clone_document_from_reader(doc)
            for page in writer.pages:
                if "/Annots" in page:
                    writer.update_page_form_field_values(page, values, auto_regenerate=False)
        save_writer(writer, args.output, args.force)
    elif args.command == "render":
        import pypdfium2 as pdfium
        if not .1 <= args.scale <= 3:
            raise ValueError("Render --scale must be between 0.1 and 3")
        out = Path(args.output)
        paths = [out / f"page-{i+1:04d}.png" for i in indices]
        for path in paths:
            destination(path, args.force)
        with pdfium.PdfDocument(args.inputs[0], password=args.password) as rendered:
            for i, target in zip(indices, paths):
                page = rendered[i]
                if page.get_width() * page.get_height() * args.scale ** 2 > 40_000_000:
                    raise ValueError("Page raster exceeds 40 million pixels")
                bitmap = page.render(scale=args.scale)
                try:
                    with target.open("wb" if args.force else "xb") as stream:
                        bitmap.to_pil().save(stream, format="PNG")
                finally:
                    bitmap.close()
                    page.close()
        print(json.dumps({"images": [str(p) for p in paths]}, ensure_ascii=False))


if __name__ == "__main__":
    try:
        if hasattr(sys.stdout, "reconfigure"):
            sys.stdout.reconfigure(encoding="utf-8")
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
