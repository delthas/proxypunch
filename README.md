# proxypunch ![Github All Releases](https://img.shields.io/github/downloads/delthas/proxypunch/total.svg?style=flat-square)

### This tool has been superseded by [autopunch](https://github.com/delthas/autopunch), a much more user-friendly and simpler tool to do the same thing! Please consider downloading and using autopunch rather than proxypunch.

**This program lets you host & connect to friends to play peer-to-peer games without having to open or redirect any port.**

*Technical details: proxypunch creates a user-friendly UDP proxy/tunnel between two peers by hole punching the user's NAT with a custom STUN-like server which additionally "matchmakes" users based on their internal port rather than their NAT-ed port.*  

## How to use

- Download the [latest version for Windows 64 bits](https://github.com/delthas/proxypunch/releases/latest/download/proxypunch.win64.exe) (for other OS check the [latest release page](https://github.com/delthas/proxypunch/releases/latest/))
- Simply double-click the downloaded executable file to run it; there is no setup or anything so put the file somewhere you'll remember
- If a Windows Security Alert popup appears, check all checkboxes and click on "Allow access"
- If prompted for an update, press `Enter` to accept the update; proxypunch will update and restart automatically
- Choose between **server** (hosting) and **client** (connecting to a host) by typing `s` or `c`, then pressing `Enter`
- Instructions continue below depending on your choice

##### Server / Hosting

- When prompted for a port, choose any port (if you're not sure, choose any randon number between 10000 and 60000), type it and press `Enter`
- In your game, start your server / start hosting on the port you chose
- Ask your peer to connect to the shown host and port, **the peer must connect with proxypunch as explained below, not directly**
- Wait for the peer to connect, then play & profit
- When you're done playing with this peer, stop hosting, and close the proxypunch window (start it again and repeat the process to play with someone else)
- Next time you run proxypunch, you can simply press `Enter` to use the same settings as last time you connected

##### Client / Connecting to a host

- Wait for the person hosting to send you a host and port to connect to
- Enter the host and port when prompted to by typing it and pressing `Enter`
- In your game, connect to the shown host and port (**not the host and port your peer gave you**) 
- Profit & play
- When you're done playing with this peer, disconnect, and close the proxypunch window (start it again and repeat the process to play with someone else)
- Next time you run proxypunch, you can simply press `Enter` to use the same settings as last time you connected

##### Troubleshooting

- If you experience any issue when restarting proxypunch to play with someone else, try to use a different port every time your run proxypunch
- If you have any other issue or feedback, either contact me on Discord at `cc#6439` or [open an issue on Github](https://github.com/delthas/proxypunch/issues/new) 

## Advanced usage

- Command-line flags are available for quick/unattended start, run `proxypunch -help` to review the flags
