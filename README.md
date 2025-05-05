[![](images/logo.png)](https://bloxroute.com)
<p align="center">
  <a href="https://github.com/bloXroute-Labs/gateway/blob/master/LICENSE.md">
    <img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="bxgateway is released under the MIT license." />
  </a>
  <a href="https://github.com/bloXroute-Labs/gateway/blob/master/CONTRIBUTING.md">
    <img src="https://img.shields.io/badge/PRs-welcome-brightgreen.svg" alt="PRs welcome!" />
  </a>
  <a href="https://discord.gg/FAYTEMm">
    <img alt="Discord" src="https://img.shields.io/discord/638409433860407300?logo=discord">  
  </a>
  <a href="https://twitter.com/intent/follow?screen_name=bloxroutelabs">
    <img alt="Twitter Follow" src="https://img.shields.io/twitter/follow/bloxroutelabs?style=social">  
  </a>
</p>

# bloXroute Gateway
The bloXroute Gateway is a blockchain client that attaches to blockchain nodes and acts as an entrypoint to bloXroute's BDN.

## What is bloXroute?
bloXroute is a blockchain scalability solution that allows all cryptocurrencies and blockchains to scale to
thousands of transactions per second (TPS) on-chain, without any protocol changes.

bloXroute solves the scalability bottleneck by addressing the substantial time required for all nodes to synchronize
when handling large volumes of TPS. Most importantly, bloXroute does this in a provably neutral way.

For more information, you can read our [white paper].

## Quick start

You can choose either to either run locally or [Docker] (recommended). Refer to  
[our technical documentation][install] for full usage instructions.

## Building the source

Building gateway requires a Go (version 1.24 or later). Once the Go environment is set up, you can build the gateway by running:

```bash
make gateway
```

## Contributing

Please read our [contributing guide] contributing guide


## Documentation

You can find our full technical documentation and architecture [on our website][documentation].

## Troubleshooting

Contact us at [our Discord] for further questions.


[white paper]: https://bloxroute.com/wp-content/uploads/2019/01/whitepaper-V1.1-1.pdf
[Docker]: https://www.docker.com
[install]: https://docs.bloxroute.com/gateway
[documentation]: https://docs.bloxroute.com/
[our Discord]: https://discord.gg/jHgpN8b
[contributing guide]: https://github.com/bloXroute-Labs/gateway/blob/master/CONTRIBUTING.md
