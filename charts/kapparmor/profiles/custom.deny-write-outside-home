profile custom.deny-write-outside-home flags=(attach_disconnected) {
  file,       # access all filesystem
  /home/** rw,
  deny /bin/** w, # deny writes in all subdirectories
  deny /etc/** w,
  deny /usr/** w,
}
