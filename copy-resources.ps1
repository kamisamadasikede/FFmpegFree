# copy-ffmpeg.ps1

Write-Output "Copying ffmpeg folder to build/bin..."

$source = ".\ffmpeg"
$destination = ".\build\bin\ffmpeg"

# 如果目标存在，先删除旧的
if (Test-Path $destination) {
    Remove-Item -Path $destination -Recurse -Force
}

# 创建目标路径（如果不存在）
New-Item -ItemType Directory -Force -Path $destination | Out-Null

# 复制 ffmpeg 文件夹
Copy-Item -Path "$source\*" -Destination $destination -Recurse

Write-Output "FFmpeg folder copied successfully."