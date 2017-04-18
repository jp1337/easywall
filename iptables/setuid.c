#include <stdlib.h>
#include <sys/types.h>
#include <unistd.h>
#include <stdio.h>

int main(void)
{
	int current_uid = getuid();
	printf("My UID is: %d. My GID is: %d\n", current_uid, getgid());
	system("/usr/bin/id");
	if (setuid(0))
	{
		perror("setuid");
		return 1;
	}
	printf("My UID is: %d. My GID is: %d\n", getuid(), getgid());
	system("/usr/bin/id");
	setuid(current_uid);
	printf("My UID is: %d. My GID is: %d\n", getuid(), getgid());
	system("/usr/bin/id");
	return 0;
}
