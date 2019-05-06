# proxypunch

**This program lets you host & connect to friends to play peer-to-peer games without having to open or redirect any port. This does not add any latency to the connection compared to redirecting ports manually.**

*Technical details: proxypunch creates a user-friendly UDP proxy/tunnel between two peers by hole punching the user's NAT with a custom STUN-like server which additionally "matchmakes" users based on their internal port rather than their NAT-ed port.*  

## News

#### 0.1.0
- Now supports multiple peers (players/spectators) when hosting
- The server window can now be kept open indefinitely (no need to restart proxypunch)

## How to use

- Download the [latest version for Windows 64 bits](https://github.com/delthas/proxypunch/releases/latest/download/proxypunch.win64.exe) (for other OS check the [latest release page](https://github.com/delthas/proxypunch/releases/latest/))
- Simply double-click the downloaded executable file to run it; there is no setup or anything so put the file somewhere you'll remember
- If a Windows Defender SmartScreen popup appears, click on "More information", then click on "Run anyway"
- If a Windows Firewall popup appears, check the "Private networks" and "Public networks" checkboxes, then click on "Allow access"
- If prompted for an update, press `Enter` to accept the update; proxypunch will update and restart automatically
- Choose between **server** (hosting) and **client** (connecting to a host) by typing `s` or `c`, then pressing `Enter`
- Instructions continue below depending on your choice

#### Server / Hosting

- When prompted for a port, choose any port (if you're not sure, choose any random number between 10000 and 60000), type it and press `Enter`
- In your game, start your server / start hosting on the port you chose
- Ask your peer to connect to the shown host and port, **the peer must connect with proxypunch as explained below, not directly**
- Wait for the peer to connect, then play & profit
- You can keep the server open and send the host and port to new players to play with them
- Next time you run proxypunch, you can simply press `Enter` to use the same settings as last time you hosted

#### Client / Connecting to a host

- Wait for the person hosting to send you a host and port to connect to; **the host and port your peer gives you must be entered in proxypunch, NOT IN THE GAME**
- Enter the host and port in proxypunch when prompted to by typing it and pressing `Enter`
- In your game, connect to the **host and port shown by proxypunch; these are DIFFERENT FROM the ones your peer gave you** *(the host is most often 127.0.0.1)*
- Profit & play
- When you're done playing with this peer, disconnect, and close the proxypunch window (start it again and repeat the process to play with someone else)
- Next time you run proxypunch, you can simply press `Enter` to use the same settings as last time you connected

### Troubleshooting

- If you experience any issue when restarting proxypunch to play with someone else, try to use a different port every time your run proxypunch
- If you have any other issue or feedback, either contact me on Discord at `cc#6439` or [open an issue on Github](https://github.com/delthas/proxypunch/issues/new) 

## Advanced usage

- Command-line flags are available for quick/unattended start, run `proxypunch -help` to review the flags
