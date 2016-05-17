FROM debian:jessie

RUN apt-get update && apt-get install -y git python-dev python-setuptools build-essential curl vim jq

RUN easy_install pip
RUN pip install paramiko PyYAML Jinja2 httplib2 six

RUN git clone git://github.com/ansible/ansible.git --recursive /opt/ansible
ENV ANSIBLE_LIBRARY /opt/ansible_modules
RUN mkdir -p $ANSIBLE_LIBRARY

RUN git clone git://github.com/google/jsonnet.git --recursive /opt/jsonnet
RUN (cd /opt/jsonnet; python setup.py install)

RUN mkdir -p /usr/share/ansible/plugins/action/
RUN curl https://raw.githubusercontent.com/mikedanese/ansible-jsonnet/1443648aabe7c2bb8357a4331d4da4415bed0046/jsonnet.py \
  -o /usr/share/ansible/plugins/action/jsonnet.py
RUN touch /opt/ansible_modules/jsonnet

ADD do_role.sh /opt/do_role.sh
CMD /opt/do_role.sh node
ADD playbooks/ /opt/playbooks/
