This utility is to try out face detection via pigo library.

# Prerequisite
1. Download all cascade files from https://github.com/esimov/pigo/blob/master/cascade/, and store them in current directory. The files would be
```
$ tree .
.
├── facefinder
├── lps
│   ├── lp312
│   ├── lp38
│   ├── lp42
│   ├── lp44
│   ├── lp46
│   ├── lp81
│   ├── lp82
│   ├── lp84
│   └── lp93
├── main.go
├── puploc
├── README.md
```

2. Usage: [/path/to/image]

 - if [/path/to/image] is single image file, it will analysis the image, extract faces and save as another files
 - if [/path/to/image] is directory, it will recursively analysis all jpg files, and save face separately.

3. Batch process:

 - Copy all preview folders to one directory, such as
```
$ mkdir -p /tmp/test-previews/alice
$ cp -r /media/USB_DISK/alice/Photos/preview/ /tmp/test-previews/alice/`
$ mkdir -p /tmp/test-previews/bob
$ cp -r /media/USB_DISK/bob/Photos/preview/ /tmp/test-previews/bob/`

```
 - Remove all webp files
```
find /tmp/test-previews -name "*.webp" | xargs -I {} rm {}
```
 - Remove lower resolution preview files
```
find /tmp/test-previews -name "*_75x0.jpg" | xargs -I {} rm {}
find /tmp/test-previews -name "*_320x0.jpg" | xargs -I {} rm {}
```
 - In case you want to remove all faces generated previously
```
find /tmp/test-previews -name "*_face*.jpg" | xargs -I {} rm {}
```
