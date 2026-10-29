"""Finder 安装窗口；dmgbuild 直接写入 .DS_Store，无需自动化 Finder。"""
from pathlib import Path

root = Path.cwd()
application = root / "build/LiveMyGo.app"

files = [str(application)]
symlinks = {"Applications": "/Applications"}
icon = str(application / "Contents/Resources/AppIcon.icns")
background = str(root / "build/installer/background.tiff")
format = "UDZO"
filesystem = "HFS+"
window_rect = ((180, 160), (640, 380))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
show_icon_preview = False
include_icon_view_settings = True
include_list_view_settings = False
icon_size = 104
text_size = 13
label_pos = "bottom"
arrange_by = None
icon_locations = {"LiveMyGo.app": (176, 190), "Applications": (464, 190)}
