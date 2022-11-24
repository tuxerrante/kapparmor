The app check continuously on a specific folder for new profiles.

The folder and the poll time can be passed as a parameter to the container and from the container to the app.

Check if the current profiles are really new:
1. check new profiles accessibility in configmap folder
2. calculate digest of new profiles
3. get the list of loaded profile names
4. calculate digests of loaded profiles  
    if the name starts with a given prefix like "custom."  
    and the definition is under /etc/apparmor.d  
    ignore it otwerwise
5. If a new profile has changed content or it is not present under /etc/apparmor.d then load it  
     and copy its definition into that folder
6. If a profile is in the 'loaded' list   
   but not in the configmap 
   and the name starts with "custom."  
   unload and delete it
