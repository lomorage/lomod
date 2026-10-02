Backend of lomorage application


[TOC]

# Arguments
```
   --base value, -b value      base directory to store db file (default: "/Users/qiwa/workspace/golang/src/bitbucket.org/lomoware/lomo-backend/cmd/lomod")
   --port value, -p value      (default: 8003)

   --help, -h                  show help
   --version, -v               print the version
```

`lomod --base /home/ubuntu/Documents --port 8080`

# Start
Mobile user should use username and password created by lomoframe and create lomoframe user on behalf of lomoframe. 
Once lomo frame user is created successfully, mobile client should signal lomoframe to start login to specified host and receive token
Note that this API doesn't require token.
No support on update existing user credential. Need more discussion
```
$ curl -s -X POST 127.0.0.1:8003/start/10.0.1.111
```

# Reset
Mobile user can always regenerate new qrcode image with new username and password
```
$ curl -s -X POST 127.0.0.1:8003/reset
```
Mobile user can also specify host url for reboot
```
$ curl -s -X POST 127.0.0.1:8003/reset/10.0.1.111
```

# System Info
System information can be queried by below API without token.
```
$ curl -s 127.0.0.1:8003/system | python -m json.tool
{
    "APIVersion": "1.0",
    "ListenIPs": {
        "wired": [
            "10.0.1.14"
        ]
    },
    "LomoFrameVersion": "284983b00a92",
    "OS": "darwin",
    "SystemStatus": 1,
    "SystemStatusLog": "",
    "MountStatus": 0,
    "MountLog": "",
    "KeepaliveStatus": 0,
    "KeepaliveLog": "",
}
```
Based on the current status, `SystemStatus` has below value

| SystemStatus | Description |
| -----------  | ----------- |
| -1           | System is abnormal   |
| 0            | New System, without any users yet   |
| 1            | System has been initialized and users are created|
| 2            | Remote lomod is not reachable |
| 3            | Login lomod fail |
| 4            | Login lomod success. It should be good status |

If `SystemStatus` is abnormal, `SystemStatusLog` will display log of potental abnormal, such as mount failure

And `MountStatus` has below value

| MountStatus | Description |
| -----------  | ----------- |
| -1           | Mount abnormal   |
| 0            | Mount success   |
| 1            | Mount in progress |
If `MountStatus` is abnormal, `MountLog` will display log of potental abnormal, such as mount failure

And `KeepaliveStatus` has below value

| KeepaliveStatus | Description |
| -----------  | ----------- |
| -1           | Keepalive abnormal   |
| 0            | Keepalive success   |
| 1            | Keepalive in progress |
If `KeepaliveStatus` is abnormal, `KeepaliveLog` will display log of potental abnormal, such as login failure

