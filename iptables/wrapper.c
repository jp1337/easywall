#include <stdlib.h>
#include <sys/types.h>
#include <unistd.h>

int
main(int argc, char *argv[])
{
	setuid(0);

	/* WARNING: Only use an absolute path to the script to execute,
	*          a malicious user might fool the binary and execute
	*          arbitary commands if not.
	* */

	system("/bin/bash /usr/local/EasyWall/iptables/timer.sh");

	return 0;
}
