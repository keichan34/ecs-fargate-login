#!/bin/bash

# Copyright (c) 2019, Keitaroh Kobayashi

# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:

# The above copyright notice and this permission notice shall be included in
# all copies or substantial portions of the Software.

# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
# THE SOFTWARE.

set -e
set -o pipefail

# Set up shell wrapper
cat << 'EOF' > /bin/shell-wrapper.sh
#!/bin/bash

set +e
/bin/bash --login "$@"
set -e
# Kill the sshd daemon when we're done here
kill -s SIGTERM 1
EOF
chmod +x /bin/shell-wrapper.sh
echo "/bin/shell-wrapper.sh" >> /etc/shells
chsh -s /bin/shell-wrapper.sh

# Set up environment for shells spawned by sshd
env | \
    grep -v '^_' | \
    grep -v '^TERM=' | \
    grep -v '^SHLVL=' | \
    grep -v '^HOME=' | \
    grep -v '^PWD=' \
    > /etc/environment

# Register authorized key
if [ -z "$_AUTHORIZED_PUBLIC_KEY" ]; then
    echo "The _AUTHORIZED_PUBLIC_KEY environment variable is not set. This is not a supported configuration, as nobody will be able to log in."
    exit 1
fi
mkdir -p $HOME/.ssh
echo -e "$_AUTHORIZED_PUBLIC_KEY" > "$HOME/.ssh/authorized_keys"
chmod 700 "$HOME/.ssh"
chmod 600 "$HOME/.ssh/authorized_keys"

echo "Authorized the following keys:"
cat "$HOME/.ssh/authorized_keys"

# SSH login fix. Otherwise user is kicked off after login
sed 's@session\s*required\s*pam_loginuid.so@session optional pam_loginuid.so@g' -i /etc/pam.d/sshd

# Set up sshd server
mkdir /var/run/sshd

echo "Booting SSH server."

exec /usr/sbin/sshd -D -e
