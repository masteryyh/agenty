package modelhub

import "golang.org/x/sys/windows"

func platformSpace(directory string) (Storage, error) {
	name, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return Storage{}, err
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(name, &available, &total, &free); err != nil {
		return Storage{}, err
	}
	volume := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(name, &volume[0], uint32(len(volume))); err != nil {
		return Storage{}, err
	}
	return Storage{Volume: windows.UTF16ToString(volume), TotalBytes: total, UsedBytes: total - free, AvailableBytes: available}, nil
}
