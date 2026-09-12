import os,signal,time,resource
resource.setrlimit(resource.RLIMIT_CORE,(0,0))
child=os.fork()
if child==0:
 signal.pause()
 os._exit(7)
time.sleep(.1)
os.kill(child,signal.SIGABRT)
_,status=os.waitpid(child,0)
assert os.WTERMSIG(status)==signal.SIGABRT
os._exit(42)
