"""ctx test paketi — ortak fixture'lar.

`bin/ctx.py` bir script (paket degil); testler onu dosya yolundan modul olarak
yukler. Paketleme (pyproject) gelince bu import tek satira duser.
"""
import importlib.util
import sys
from pathlib import Path

import pytest

KOK = Path(__file__).resolve().parent.parent


def _ctx_yukle():
    yol = KOK / "bin" / "ctx.py"
    spec = importlib.util.spec_from_file_location("ctx_modul", yol)
    modul = importlib.util.module_from_spec(spec)
    sys.modules["ctx_modul"] = modul
    spec.loader.exec_module(modul)
    return modul


ctx = _ctx_yukle()


@pytest.fixture
def ctx_modul():
    return ctx


def not_yaz(hafiza: Path, goreli: str, *, title=None, description="Aciklama",
            status="active", scope=None, lifecycle=None, type_="domain",
            tags=None, components=None, updated="2026-08-13",
            superseded_by="null", govde="Govde metni."):
    """Test notu olusturur. Frontmatter alanlari ctx'in bekledigi sirada."""
    p = hafiza / goreli
    p.parent.mkdir(parents=True, exist_ok=True)
    # Baslik verilmediyse dosya adindan turet: iki notun sessizce ayni
    # basligi tasimasi "baslik tekrari" uyarisi uretir ve testleri kirlendirir.
    if title is None:
        title = p.stem.replace("-", " ").title()
    satirlar = [
        "---",
        f"title: {title}",
        f'description: "{description}"',
        f"type: {type_}",
    ]
    # Eski tek-eksenli alan yalniz ACIKCA istendiginde yazilir (geriye
    # uyumluluk testleri icin); varsayilan yeni modeldir.
    if lifecycle is not None:
        satirlar.append(f"lifecycle: {lifecycle}")
    else:
        satirlar.append(f"status: {status}")
        if scope:
            satirlar.append(f"scope: [{scope if isinstance(scope, str) else ', '.join(scope)}]")
    if tags:
        satirlar.append(f"tags: [{', '.join(tags)}]")
    if components:
        satirlar.append(f"components: [{', '.join(components)}]")
    satirlar += [
        f"updated: {updated}",
        f"superseded_by: {superseded_by}",
        "---",
        "",
        govde,
        "",
    ]
    p.write_text("\n".join(satirlar), encoding="utf-8")
    return p


@pytest.fixture
def hafiza(tmp_path):
    """Bos hafiza iskeleti (ctx'in bekledigi klasorler)."""
    for klasor in ("domain", "architecture", "decisions", "pitfalls", "tasks", "sessions"):
        (tmp_path / klasor).mkdir()
    (tmp_path / "README.md").write_text("# test hafiza\n", encoding="utf-8")
    (tmp_path / "tasks" / "OPEN.md").write_text("# acik isler\n", encoding="utf-8")
    (tmp_path / "sessions" / "CURRENT.md").write_text("# nerede kaldik\n", encoding="utf-8")
    return tmp_path


@pytest.fixture
def yaz():
    return not_yaz
