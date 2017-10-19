# ------------------------------------------------------------------------------------------------------------------------
# We are explicitly not using a templating language to inject the values as to encourage the user to limit their
# use of templating logic in these files. By design all injected values should be able to be set at runtime,
# and the shell script real work. If you need conditional logic, write it in bash or make another shell script.
# ------------------------------------------------------------------------------------------------------------------------
OPENVPN_KEYCOUNTRY="US"
OPENVPN_KEYPROVINCE="CA"
OPENVPN_KEYCITY="SanFrancisco"
OPENVPN_KEYORG="Kubicorn"
OPENVPN_KEYEMAIL="root@localhost"
OPENVPN_KEYOU="Kubicorn"
OPENVPN_KEYNAME="server"

PRIVATE_IP=$(curl http://169.254.169.254/metadata/v1/interfaces/private/0/ipv4/address)

# OpenVPN
yum install epel-release -y
yum install openvpn easy-rsa -y

mkdir ~/openvpn-ca
cp -rf /usr/share/easy-rsa/2.0/* ~/openvpn-ca/

sed -i -e "s/export KEY_COUNTRY.*/export KEY_COUNTRY=\"${OPENVPN_KEYCOUNTRY}\"/" ~/openvpn-ca/vars
sed -i -e "s/export KEY_PROVINCE.*/export KEY_PROVINCE=\"${OPENVPN_KEYPROVINCE}\"/" ~/openvpn-ca/vars
sed -i -e "s/export KEY_CITY.*/export KEY_CITY=\"${OPENVPN_KEYCITY}\"/" ~/openvpn-ca/vars
sed -i -e "s/export KEY_ORG.*/export KEY_ORG=\"${OPENVPN_KEYORG}\"/" ~/openvpn-ca/vars
sed -i -e "s/export KEY_EMAIL.*/export KEY_EMAIL=\"${OPENVPN_KEYEMAIL}\"/" ~/openvpn-ca/vars
sed -i -e "s/export KEY_OU.*/export KEY_OU=\"${OPENVPN_KEYOU}\"/" ~/openvpn-ca/vars
sed -i -e "s/export KEY_NAME.*/export KEY_NAME=\"${OPENVPN_KEYNAME}\"/" ~/openvpn-ca/vars

## Generate server certificates
cd ~/openvpn-ca
source vars
./clean-all
./build-ca --batch
./build-key-server --batch ${OPENVPN_KEYNAME}
./build-dh
openvpn --genkey --secret keys/ta.key

## Generate client certificates
./build-key --batch clients

## Generate OpenVPN configuration
cp ~/openvpn-ca/keys/ca.crt ~/openvpn-ca/keys/ca.key ~/openvpn-ca/keys/${OPENVPN_KEYNAME}.crt \
    ~/openvpn-ca/keys/${OPENVPN_KEYNAME}.key ~/openvpn-ca/keys/ta.key ~/openvpn-ca/keys/dh2048.pem /etc/openvpn
cp /usr/share/doc/openvpn-2.4.3/sample/sample-config-files/server.conf /etc/openvpn/${OPENVPN_KEYNAME}.conf

### Adjust TLS configuration
sed -i -e "s/\;tls-auth ta.key 0.*/tls-auth ta.key 0/" /etc/openvpn/${OPENVPN_KEYNAME}.conf
sed -i -e "/tls-auth ta.key 0/a key-direction 0" /etc/openvpn/${OPENVPN_KEYNAME}.conf

### Enable AES-128-CBC chipers
sed -i -e "s/\;cipher AES-128-CBC.*/cipher AES-128-CBC/" /etc/openvpn/${OPENVPN_KEYNAME}.conf
sed -i -e "/cipher AES-128-CBC/a auth SHA256" /etc/openvpn/${OPENVPN_KEYNAME}.conf

### Set user and group
sed -i -e "s/\;user nobody.*/user nobody/" /etc/openvpn/${OPENVPN_KEYNAME}.conf
sed -i -e "s/\;group nogroup.*/group nogroup/" /etc/openvpn/${OPENVPN_KEYNAME}.conf

### TODO(xmudrii): find way to generate new cert for every client
### Enable duplicate certificates
sed -i -e "s/\;duplicate-cn.*/duplicate-cn/" /etc/openvpn/${OPENVPN_KEYNAME}.conf

## Enable IP forwarding
sed -i -e "s/\#net.ipv4.ip_forward.*/net.ipv4.ip_forward=1/" /etc/sysctl.conf
sysctl -p

systemctl start openvpn@${OPENVPN_KEYNAME}
systemctl enable openvpn@${OPENVPN_KEYNAME}

## Generate client configuration

### Create the directory structure and secure it
mkdir -p ~/client-configs/files
chmod 700 ~/client-configs/files

### Generate config from examples
cp /usr/share/doc/openvpn-2.4.3/sample/sample-config-files/client.conf ~/client-configs/base.conf

### Add remote IP address
sed -i -e "s/remote my-server-1 1194/remote ${PRIVATE_IP} 1194/" ~/client-configs/base.conf

### Set nobody:nogroup to run OpenVPN
sed -i -e "s/\;user nobody.*/user nobody/" ~/client-configs/base.conf
sed -i -e "s/\;group nogroup.*/group nogroup/" ~/client-configs/base.conf

### Make it not use default certificate
sed -i -e "s/ca ca.crt/\#ca ca.crt/" ~/client-configs/base.conf
sed -i -e "s/cert client.crt/\#cert client.crt/" ~/client-configs/base.conf
sed -i -e "s/key client.key/\#key client.key/" ~/client-configs/base.conf

### Configure chipers
sed -i -e "s/\;cipher x.*/cipher AES-128-CBC/" ~/client-configs/base.conf
sed -i -e "/cipher AES-128-CBC/a auth SHA256" ~/client-configs/base.conf

### Additional settings
echo "key-direction 1" >> ~/client-configs/base.conf
echo "script-security 2" >> ~/client-configs/base.conf

## Generate keys
KEY_DIR=~/openvpn-ca/keys
OUTPUT_DIR=/tmp
BASE_CONFIG=~/client-configs/base.conf

cat ${BASE_CONFIG} \
    <(echo -e '<ca>') \
    ${KEY_DIR}/ca.crt \
    <(echo -e '</ca>\n<cert>') \
    ${KEY_DIR}/clients.crt \
    <(echo -e '</cert>\n<key>') \
    ${KEY_DIR}/clients.key \
    <(echo -e '</key>\n<tls-auth>') \
    ${KEY_DIR}/ta.key \
    <(echo -e '</tls-auth>') \
    > ${OUTPUT_DIR}/clients.conf



