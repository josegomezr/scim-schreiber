FROM registry.suse.com/suse/389-ds:2.5
RUN cp /usr/lib/dirsrv/dscontainer /usr/lib/dirsrv/dscontainer.orig \
    && sed -i 's/ds_proc.wait()/pass/' /usr/lib/dirsrv/dscontainer \
    && /usr/lib/dirsrv/dscontainer -r \
    && mv /usr/lib/dirsrv/dscontainer.orig /usr/lib/dirsrv/dscontainer \
    && cp -r /data/ /base/
    
RUN echo -e '#!/bin/sh\ncp -r /base/* /data/\n/usr/lib/dirsrv/dscontainer -r' > /entrypoint.sh && chmod +x /entrypoint.sh

ENTRYPOINT /entrypoint.sh
