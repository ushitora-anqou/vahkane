let handler =
  let open Yume.Server in
  Router.(use [ get "/" (fun _ _ -> respond "hello") ]) default_handler

let () =
  Eio_main.run @@ fun env ->
  Eio.Switch.run @@ fun sw ->
  let listen =
    Eio.Net.getaddrinfo_stream ~service:"38000" env#net "localhost" |> List.hd
  in
  Yume.Server.start_server env ~sw ~listen handler @@ fun _socket -> ()
