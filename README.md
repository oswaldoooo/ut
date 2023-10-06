# UT(Udp And Tcp Multi Socket) Document

### First, Accept
The same as tcp, accept tcp connection, then wait udp connection. After get udp address from udp register message, the ut connection was complete here.
### Second, Receive Message
Ut Connection receive message is different from normal tcp/udp. It's receive tcp and udp message both. So there is very very important on delay time. We use **delay time** to **differentiate between tcp and udp message**.
### Third, Close Connection
If you just use, don't care this step. If you want to 