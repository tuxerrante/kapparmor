

```bash
# Set VM hostname as valid hostname name (no capital cases like in the default "yourname@yourname-VirtualBox")
hostnamectl hostname virtualbox
restart shell
enable cgroups
sudo apt-get -y install cgroup-tools
stat -c %T -f /sys/fs/cgroup
cat /proc/cgroups
restart

# Install microk8s and check the status
sudo snap install microk8s --classic --channel=latest/stable
microk8s inspect
aa-status |grep microk8s

# Check if the current machine results as an active node with apparmor enabled
microk8s kubectl get nodes -o=jsonpath='{range .items[*]}{@.metadata.name}: {.status.conditions[?(@.reason=="KubeletReady")].message}{"\n"}{end}'
```

Verify pods have some syscall blocked

`kubectl run -n dev-testing amicontained --rm -it --image=jess/amicontained -- amicontained`  

    Container Runtime: kube
    Has Namespaces:
            pid: true
            user: false
    AppArmor Profile: cri-containerd.apparmor.d (enforce)
    Capabilities:
            BOUNDING -> chown dac_override fowner fsetid kill setgid setuid setpcap net_bind_service net_raw sys_chroot mknod audit_write setfcap
    Seccomp: disabled
    Blocked Syscalls (24):
            MSGRCV SYSLOG SETPGID SETSID VHANGUP PIVOT_ROOT ACCT SETTIMEOFDAY UMOUNT2 SWAPON SWAPOFF REBOOT SETHOSTNAME SETDOMAINNAME INIT_MODULE DELETE_MODULE LOOKUP_DCOOKIE KEXEC_LOAD PERF_EVENT_OPEN FANOTIFY_INIT OPEN_BY_HANDLE_AT FINIT_MODULE KEXEC_FILE_LOAD BPF

## Test profiles

deny_all_writes.profile  

    profile deny-write flags=(attach_disconnected) {
    file,       # access all filesystem
    deny /** w, # deny writes in all root subdirectories
    }

deny_all_writes_outside_home.profile

    profile deny-write flags=(attach_disconnected) {
    file,
    /home/** rw,
    deny /bin/** w,
    deny /etc/** w,
    deny /usr/** w,
    }

busybox_dontwrite_pod.yml
```yml
apiVersion: v1
kind: Pod
metadata:
  name: busy-dontwrite
  annotations:
    container.apparmor.security.beta.kubernetes.io/busy-dontwrite: localhost/deny-write
spec:
  containers:
  - name: busy-dontwrite
    image: busybox
    command: [ "sh", "-c", "echo 'Hello AppArmor!' && sleep 1h" ]
    resources: {}
  restartPolicy: Always
```

```bash
# Run the pod on the node withou profile and check if it fails
microk8s kubectl apply -f busybox_dontwrite_pod.yml
microk8s kubectl get pods -o wide
microk8s kubectl get events --sort-by=.lastTimestamp

# Parse the profile on the node and check the pod status
sudo apparmor_parser --preprocess -v deny_all_writes_profile 
sudo less /sys/kernel/security/apparmor/profiles

# Try to write from the profiled pod
microk8s kubectl exec busy-dontwrite -- cat /proc/1/attr/current
microk8s kubectl exec busy-dontwrite -- touch /home/fail

# Parse the Updated profile on the node and check the pod status
sudo apparmor_parser --replace -v deny_all_writes_outside_home.profile

docker run --name tracee --rm --privileged -v /lib/modules/:/lib/modules/:ro -v /usr/src:/usr/src:ro -v /tmp/tracee:/tmp/tracee -it aquasec/tracee:0.4.0 --trace container=new

microk8s kubectl exec busy-dontwrite -- touch /home/dont_fail_please
```