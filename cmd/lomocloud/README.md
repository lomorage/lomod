Backend of lomorage application


[TOC]

# Arguments
```
   --base value, -b value    base directory to store db file (default: "/go/src/bitbucket.org/lomoware/lomo-backend")
   --domain value, -d value  domain name for DDNS (default: "hub.lomorage.com")
   --port value, -p value    (default: 8002)
   --help, -h                show help
   --version, -v             print the version
```

`lomocloud--base /home/ubuntu/Documents --port 8082`


# IP Mapping
## Set IP mapping for current public IP
```
$ cat ipmap.json
[{"MAC": "ESIzRFVm", "PrivateIP": "10.10.10.1"}, {"MAC": "ZiIzRFVm", "PrivateIP": "10.100.100.1" } ]
$ curl -s -X POST --data-binary @./ipmap.json 127.0.0.1:8002/account/ip_map
```
## List current IP mapping under current public IP
```
 curl -s -X GET 127.0.0.1:8002/account/ip_map | python -m json.tool
[
    {
        "MAC": "ESIzRFVm",
        "PrivateIP": "10.10.10.1",
        "Port": 8000,
        "PublicIP": "127.0.0.1",
        "UUID": "e4fd830d-8d75-46aa-9870-b2fc405f81e7"
    },
    {
        "MAC": "ZiIzRFVm",
        "PrivateIP": "10.100.100.1",
        "Port": 8000,
        "PublicIP": "127.0.0.1",
        "UUID": "43e229b6-de2a-48de-aa8c-029682169c13"
    }
]
```
## Update ip mapping
If one device's both public IP and private IP are changed, use same POST api will update record in the DB. For example, run the `curl` command at another machine:
```
$ cat ipmap.json
[{"MAC": "ESIzRFVm", "PrivateIP": "192.192.192.1"}, {"MAC": "ZiIzRFVm", "PrivateIP": "192.10.10.1" } ]
$ curl -s -X GET 10.0.1.13:8002/account/ip_map | python -m json.tool
[
    {
        "MAC": "ESIzRFVm",
        "PrivateIP": "192.192.192.1",
        "Port": 8000,
        "PublicIP": "10.0.1.15",
        "UUID": "e4fd830d-8d75-46aa-9870-b2fc405f81e7"
    },
    {
        "MAC": "ZiIzRFVm",
        "PrivateIP": "192.10.10.1",
        "Port": 8000,
        "PublicIP": "10.0.1.15",
        "UUID": "43e229b6-de2a-48de-aa8c-029682169c13"
    }
]
```
If one device's private IP only change, user same POST api will update private IP only.
```
$ cat ipmap.json
[{"MAC": "ESIzRFVm", "PrivateIP": "192.168.100.1"}, {"MAC": "ZiIzRFVm", "PrivateIP": "10.10.10.1" } ]
$ curl -s -X POST --data-binary @./ipmap.json 10.0.1.13:8002/account/ip_map
$ curl -s -X GET 10.0.1.13:8002/account/ip_map | python -m json.tool
[
    {
        "MAC": "ESIzRFVm",
        "PrivateIP": "192.168.100.1",
        "PublicIP": "10.0.1.15"
    },
    {
        "MAC": "ZiIzRFVm",
        "PrivateIP": "10.10.10.1",
        "PublicIP": "10.0.1.15"
    }
]
```
