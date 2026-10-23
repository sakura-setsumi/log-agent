# 栈桥正式品牌资源

采用用户确认的「桥拱与数据流」版本。原始确认图保存在 zhanqiao-logo-approved.png。

- brand-logo.png：完整品牌标志，保留原设计及白色底。
- favicon.png：应用中的圆角图标，边角透明。
- app-icon.ico：Windows 多尺寸圆角图标，边角透明。
- zhanqiao-app-icon.png：512 × 512 圆角图标源图，圆角半径 136 像素，四周保留 24 像素透明留白，便于在任务栏小尺寸下识别圆角。

使用 scripts/export-brand.py（依赖 Pillow）裁切并导出资源，不重绘标志。
Windows 资源更新命令见项目 README.md。构建输出为 dist/栈桥.exe。