"""Export application assets from the approved logo without redrawing it.

Run from any directory with Python and Pillow installed.
"""
from pathlib import Path
from PIL import Image, ImageChops

root = Path(__file__).resolve().parents[1]
source = Image.open(root / "assets/branding/zhanqiao-logo-approved.png").convert("RGB")


def artwork_bounds(image):
    difference = ImageChops.difference(image, Image.new("RGB", image.size, "white"))
    return difference.convert("L").point(lambda value: 255 if value > 12 else 0).getbbox()


bounds = artwork_bounds(source)
logo = source.crop((bounds[0] - 16, bounds[1] - 16, bounds[2] + 16, bounds[3] + 16))
logo.save(root / "brand-logo.png")
# The approved horizontal lockup places its bridge symbol left of x=400.
symbol = source.crop((0, 0, 400, source.height))
symbol = symbol.crop(artwork_bounds(symbol))
symbol.thumbnail((440, 440), Image.Resampling.LANCZOS)
icon = Image.new("RGB", (512, 512), "white")
icon.paste(symbol, ((512 - symbol.width) // 2, (512 - symbol.height) // 2))
icon.save(root / "assets/branding/zhanqiao-app-icon.png")
icon.resize((128, 128), Image.Resampling.LANCZOS).save(root / "favicon.png")
icon.save(root / "app-icon.ico", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
print("Exported brand-logo.png, favicon.png, app-icon.ico and application icon PNG")