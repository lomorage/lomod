# fs-over-http
manager remote directory over http

## Arguments
   --base value, -b value  (default: "/home/ubuntu")
   --port value, -p value  (default: 8000)

`fs-over-http --base /home/ubuntu/Documents --port 8080`

## Browse Folders

Type:

 - 0 - directory
 - 1 - file

```
$ !curl
curl -s http://127.0.0.1:8000/ | python -m json.tool
{
    "Files": [
        {
            "Name": "Documents",
            "Type": 0
        },
        {
            "Name": "Movies",
            "Type": 0
        },
        {
            "Name": "Musics",
            "Type": 0
        },
        {
            "Name": "Photos",
            "Type": 0
        },
        {
            "Name": "test.m4a",
            "Type": 1
        }
    ]
}
```
## Download a file

`$ wget http://127.0.0.1:8000/test.m4a`

## Upload file

`curl  -X POST --data-binary @video.mp4 http://127.0.0.1:8000/video.mp4`

```
$ curl -s http://127.0.0.1:8000/ | python -m json.tool
{
    "Files": [
        {
            "Name": "Documents",
            "Type": 0
        },
        {
            "Name": "Movies",
            "Type": 0
        },
        {
            "Name": "Musics",
            "Type": 0
        },
        {
            "Name": "Photos",
            "Type": 0
        },
        {
            "Name": "test.m4a",
            "Type": 1
        },
        {
            "Name": "video.mp4",
            "Type": 1
        }
    ]
}
```

## Create folders
```
$ curl -X PUT -s http://127.0.0.1:8000/Documents/travel
$ curl -s http://127.0.0.1:8000/Documents | python -m json.tool
{
    "Files": [
        {
            "Name": "travel",
            "Type": 0
        },
    ]
}
```

## Delete a file or a folder
`$ curl -X DELETE -s http://127.0.0.1:8000/Documents/travel`
