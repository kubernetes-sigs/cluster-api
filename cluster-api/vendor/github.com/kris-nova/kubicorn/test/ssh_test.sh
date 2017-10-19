DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

docker build -t eg_sshd "${DIR}"
docker run --rm -p 6666:22 -d --name test_sshd eg_sshd