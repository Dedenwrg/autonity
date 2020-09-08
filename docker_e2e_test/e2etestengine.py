#!/usr/bin/env/ python3
import argparse

import log
from conf import conf
from testcase.testcase import TestCase
from planner.networkplanner import NetworkPlanner
from client.client import Client
import time
import copy

LG = log.get_logger()


if __name__ == '__main__':
    LG.debug("##########################################")
    LG.debug("")
    LG.debug("")
    LG.debug("Test Engine start.")

    parser = argparse.ArgumentParser()
    parser.add_argument("autonity", help='Autonity Binary Path')
    parser.add_argument("-d", help='Start deploy remote network with brand new configurations.', type=bool, default=True)
    parser.add_argument("-t", help='Start test remote network.', type=bool, default=True)
    parser.add_argument("-l", help='Start and run a long run test.', type=bool, default=False)
    args = parser.parse_args()

    is_deploy = args.d
    is_testing = args.t
    is_long_run = args.l
    autonity_path = args.autonity

    conf.load_project_conf()
    network_planner = None
    passed_test_cases = []
    failed_test_cases = []
    test_cases = []

    exit_code = 0
    num_of_cases = 0

    # Deploy will create brand new configurations then bootstrap entire network from genesis block.
    if is_deploy:
        network_planner = NetworkPlanner(autonity_path)
        network_planner.plan()
        network_planner.deploy()
        network_planner.start_all_nodes()

    if is_testing:
        clients = None
        if network_planner:
            clients = network_planner.get_clients()
        else:
            # load network view from generated testbed.conf
            clients = []
            test_bed = conf.get_test_bed_conf()
            try:
                for node in test_bed["targetNetwork"]["nodes"]:
                    client = Client(host=node["name"], p2p_port=node["p2pPort"], rpc_port=node["rpcPort"],
                                    ws_port=node["wsPort"], graphql_port=node["graphqlPort"],
                                    net_interface=node["ethernetInterfaceID"], coin_base=node["coinBase"][2:],
                                    ssh_user=node["sshCredential"]["sshUser"], ssh_pass=node["sshCredential"]["sshPass"],
                                    ssh_key=node["sshCredential"]["sshKey"], sudo_pass=node["sshCredential"]["sudoPass"],
                                    role=node["role"], index=node["index"])
                    clients.append(client)
            except Exception as e:
                LG.error("Process exit with cannot conf from test bed conf.", e)
                exit_code = 1

        try:
            # load test case view, and start testing one by one.
            test_set = conf.get_test_case_conf()
            num_of_cases = len(test_set["playbook"]["testcases"])
            test_cases = copy.deepcopy(test_set["playbook"]["testcases"])
            if is_long_run is True:
                num_of_cases = len(test_set["playbook"]["longcases"])
                test_cases = copy.deepcopy(test_set["playbook"]["longcases"])
            for test_case in test_cases:
                playbook = conf.get_test_case_conf()
                if playbook["playbook"]["stop"] is True:
                    LG.info("Playbook is stopped by user configuration: testcaseconf.yml/playbook/stop: true.")
                    break
                test = TestCase(test_case, clients)
                LG.debug("\n\n\n")
                LG.info("start test case: %s", test_case)
                LG.debug("\n\n\n")
                result = test.start_test()
                if result is True:
                    LG.info('TEST CASE PASSED: %s', test_case)
                    passed_test_cases.append(test_case)
                if result is False:
                    LG.error('TEST CASE FAILED: %s', test_case)
                    failed_test_cases.append(test_case)

        except (KeyError, TypeError) as e:
            LG.error("Wrong configuration. %s", e)
            exit_code = 1
        except Exception as e:
            LG.error("Get error: %s", e)
            exit_code = 1

    # generate an overview of the test report.
    if len(passed_test_cases) == num_of_cases:
        LG.info("[TEST PASSED]")

    LG.info("[PASS] %d/%d cases were passed.", len(passed_test_cases), num_of_cases)
    for case in passed_test_cases:
        LG.info("[PASS] %s", case["name"])

    if len(failed_test_cases) > 0:
        exit_code = 1
        LG.info("[TEST FAILED]")
        LG.info("[FAILED] %d/%d cases were failed.", len(failed_test_cases), num_of_cases)
        for case in failed_test_cases:
            LG.info("[ERROR] %s", case["name"])

        for i in range(0, len(failed_test_cases)):
            LG.info("Log collecting...")
            time.sleep(180)

    exit(exit_code)
