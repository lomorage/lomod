# exif-app
Simple application to display picture's EXIF information

## Arguments
   exif-app <filename>

```
$./exif-app ../v2/test/img/true_2003_11_23.jpg
...
DateTime: "2005:07:02 10:38:28"
ExposureBiasValue: "0/6"
DigitalZoomRatio: "1/1"
...
```

## Notes
This application only reads exif metadata, to manipulate exif metadata, exiftool can be used. for example,

###Set DateTimeOriginal to Any Arbitrary Timestamp
`exiftool '-datetimeoriginal=2015:01:18 12:00:00' sample.jpg`

Refer https://gist.github.com/rjames86/33b9af12548adf091a26


