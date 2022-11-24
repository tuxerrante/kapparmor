profile custom.deny-write-outside-app flags=(attach_disconnected) {
  file,       # access all filesystem
  /app/** rw,
  deny /bin/** w, # deny writes in all subdirectories
  deny /etc/** w,
  deny /usr/** w,
}