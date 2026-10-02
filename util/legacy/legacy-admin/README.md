# admin protocol
Administration API. It sits at different port, and only admin can access

## Arguments
   --base value, -b value  (default: "/home/ubuntu")
   --port value, -p value  (default: 8001)
   --mount value, -m value  (default: "/mnt/lomo")

`admin --base /home/ubuntu/Documents --port 8001 --mount /mnt/lomo`

## USB Mount management

### list mounts. Initially no any mount
```
$curl 127.0.0.1:8001/mount/usb
[]
```
### Plug in one USB, list mount will show the device, but not in mounted state

```
$ curl 127.0.0.1:8001/mount/usb
[{"UUID":"6326-1619","Name":"sda1","Mounted":false,"Total":"3.72G","Free":"","Path":""}]

```
### Mount the device
Mount usb device. Base mount path will be returned, and client can use this path to umount or browse.
```
$curl -X POST 127.0.0.1:8001/mount/usb/sda1
6326-1619
```

list mount again will show the right status
```
$ curl 127.0.0.1:8001/mount/usb
[{"UUID":"6326-1619","Name":"sda1","Mounted":true,"Total":"3.72G","Free":"3.72G","Path":"/mnt/lomo/6326-1619"}]
```
### Unmount the device
```
curl -X DELETE 127.0.0.1:8001/umount/6326-1619
```
list device again, and device should be in unmount state
```
$ curl 127.0.0.1:8001/mount/usb
[{"UUID":"6326-1619","Name":"sda1","Mounted":false,"Total":"3.72G","Free":"","Path":""}]

```
## Remote Samba Mount management
Mount remote samba folder. Base mount path will be returned, and client can use this path to umount or browse.
```
$ curl -X POST "127.0.0.1:8001/mount/smb?url=vagrant:vagrant@192.168.33.10/share"

"192.168.33.10_share"
```
Unmount
```
$ curl -X DELETE "127.0.0.1:8001/umount/192.168.33.10_share"
```

## Import management
Browse mounted directory. This has same structure as v1 API.
Assuming 192.168.33.10_share is new mounted folder. Below is one example.
```
$ curl -s 127.0.0.1:8001/browse/192.168.33.10_share | python   -m json.tool
  {
      "Files": [
          {
             "Name": "2003",
             "Type": 0
          },
          {
              "Name": "2004",
              "Type": 0
          }
      ]
  ٝ}
$ curl -s 127.0.0.1:8001/browse/192.168.33.10_share/2003 | python   -m json.tool
{
    "Files": [
        {
            "Name": "1",
            "Type": 0
        },
        {
            "Name": "11",
             "Type": 0
        }
    ]
ٝ}
$ curl -s 127.0.0.1:8001/browse/192.168.33.10_share/2003/1 | python   -m json.tool
{
    "Files": [
        {
            "Name": "17",
            "Type": 0
        }
    ]
}
$ curl -s 127.0.0.1:8001/browse/192.168.33.10_share/2003/1/17 | python   -m json.tool
ٝ{
    "Files": [
        {
            "Name": "4.jpg",
            "Type": 1
        }
    ]
ٝ}

```
After you know where you want to start import, you can run to import all images under this folder.
For example, below request will import all images under this folder.
If duplicate or bad images or videos are found, will proceed and continue to import next ones.
```
$ curl -s 127.0.0.1:8001/import/192.168.33.10_share
```

## User database management

### init database
After mounting the device, admin can init user database which including assets management. It is v2 app can use and specify at start
This init command will requires user and password, and will set add this user and set the user as admin.
URL need specify which mount device as initial storage. In the future, if user wants to change different device, they can use user update command to change home dir
```
curl -X POST "192.168.1.137:8001/init/sda1?username=qiwa&password=qiwa"
```

### list new user
This commands are same as v2 API. Please refer that documents.
List user will list existing users.
```
curl "192.168.1.137:8001/user?username=qiwa&token=573944500"
{"Name":["qiwa"]}
```
### add new user
format is <http host>/user/<user name>/<device name>/<password>?username=<admin user>&token=<token>
```
curl -X POST "192.168.1.137:8001/user/test/sda1/test123?username=qiwa&token=573944500"
```
List user will list the new user.
```
curl "192.168.1.137:8001/user?username=qiwa&token=573944500"
{"Name":["qiwa", "test"]}
```

Now user can use v2 API to upload assets.

## Import from exists devices
### scan remote share in the network
```
$ curl -s 127.0.0.1:8001/scan | python -m json.tool
[
    {
        "Ip": "192.168.1.130",
        "Name": "AAA",
        "Port": 445
    },
    {
        "Ip": "192.168.1.130",
        "Name": "AAA",
        "Port": 548
    }
]

```
### connect remote share
