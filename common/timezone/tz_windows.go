package timezone

// refer https://socketloop.com/tutorials/golang-display-list-of-timezones-with-gmt

// List available time zone in the system
func List() ([]string, error) {
	return nil, nil
	/*
		//k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Time Zones`, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)

		//if err != nil {
		// fmt.Println(err)
		//}
		//defer k.Close()

		//names, err := k.ReadSubKeyNames(-1)
		//if err != nil {
		// fmt.Println(err)
		//}

		//fmt.Println("Number of timezones : ", len(names))
		//for i := 0; i <= len(names)-1; i++ {
		// check if tz is already in timeZones slice
		// append if not
		// if !InSlice(names[i], timeZones) { // need a more efficient method...
		//  timeZones = append(timeZones, names[i])
		// }
		//}

		// UPDATE : Reading from registry is not reliable
		// better to parse output result by "tzutil /g" command
		// REMEMBER : There is no time difference between Coordinated Universal Time and Greenwich Mean Time ....
		cmd := exec.Command("tzutil", "/l")

		data, err := cmd.Output()

		if err != nil {
			return nil, err
		}

		GMTed := bytes.Replace(data, []byte("UTC"), []byte("GMT"), -1)

		now := time.Now()

		for _, v := range timeZones {

			if runtime.GOOS != "windows" {

				location, err := time.LoadLocation(v)
				if err != nil {
					fmt.Println(err)
				}

				// extract the GMT
				t := now.In(location)
				t1 := fmt.Sprintf("%s", t.Format(time.RFC822Z))
				tArray := strings.Fields(t1)
				gmtTime := strings.Join(tArray[4:], "")
				hours := gmtTime[0:3]
				minutes := gmtTime[3:]

				gmt := "GMT" + fmt.Sprintf("%s:%s", hours, minutes)
				fmt.Println(gmt + " " + v)

			} else {
				fmt.Println(v)
			}

		}
	*/
}
