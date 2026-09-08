process.on("SIGTERM", () => {});
if (process.send) process.send("ready");
setInterval(() => {}, 1000);
