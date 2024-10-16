# Copyright 2022 University of California, Riverside
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0

#!/usr/bin/python

import time, socket, os, sys
import argparse, logging, yaml
from multiprocessing import shared_memory
from shared_memory_dict import SharedMemoryDict

DEFAULT_LOG_LEVEL='info'
logger = logging.getLogger(__name__)

class SharedMemoryManager(object):
    def __init__(self, shm_block_count, shm_block_size, shm_free_pool_dict_name):
        self.shm_block_count = shm_block_count # number of SHM blocks
        self.shm_block_size = shm_block_size # size of each SHM block
        self.shm_free_pool_dict_name = shm_free_pool_dict_name # name of the shared mem dict having free shm blocks

        self.shm_free_pool_dict_size = shm_block_count * 32  # 32 bytes for each dict entry

        self.shm_free_pool_dict, self.shm_dict_all = self.create_shm_pool(self.shm_block_count, \
                                                                          self.shm_block_size, \
                                                                          self.shm_free_pool_dict_name)

    def create_shm_pool(self, num_of_blocks, block_size, shm_free_pool_dict_name):
        logger.info("Creating {} blocks of {} bytes each...".format(num_of_blocks, block_size))

        # NOTE: sm_dict_free contains set of shm blocks that are free at any given point
        sm_dict_free = SharedMemoryDict(name = shm_free_pool_dict_name, size = num_of_blocks * 32)
        # NOTE: sm_dict_all contains all shm blocks, used only to free up all shm blocks during sys exit
        sm_dict_all = SharedMemoryDict(name = "sm_dict_all", size = num_of_blocks * 32)

        for i in range(num_of_blocks):
            if i % 1000 == 0:
                logger.info("{} block created".format(i))
            temp_block = shared_memory.SharedMemory(create = True, size = block_size)
            sm_dict_free[str(temp_block.name)] = 'FREE'
            sm_dict_all[str(temp_block.name)] = ''
        
        return (sm_dict_free, sm_dict_all)

    def createSocket(self):
        try:
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        except socket.error as msg:
            print('Failed to create socket. Error code: {}, Error message: {}'.format(str(msg[0]), msg[1]))
            sys.exit()

        return sock

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='SHM Manager')
    parser.add_argument('--config-file', help='Path of the config file')
    parser.add_argument('--block-count', help='no of shm blocks', default = 10)
    parser.add_argument('--block-size', help='size of each shm block', default = 8)
    parser.add_argument('--pool-name', help='name of shm free dict', default = 'shm_free_dict')
    parser.add_argument('--log-level', help='Log level', default = DEFAULT_LOG_LEVEL)
    args = parser.parse_args()
    logging.basicConfig(format='%(asctime)s - %(name)s - %(levelname)s - %(message)s', level=logging.getLevelName(args.log_level.upper()))

    with open(args.config_file) as config_file:
        config = yaml.load(config_file)
        logger.debug("Config %s", config)
        spright_cp_config = config['spright_control_plane']

    logger.debug('Creating shared memory...')
    sm_mgr = SharedMemoryManager(shm_block_count = int(args.block_count), \
                                 shm_block_size = int(args.block_size), \
                                 shm_free_pool_dict_name = args.pool_name)

    logger.debug('Starting socket in SMM server...')
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.bind((spright_cp_config['smm_server_ip'], spright_cp_config['smm_server_port'])) 
    s.listen()
    
    while True:
        try:
            conn, addr = s.accept()
            logger.debug('Got connection from {}'.format(addr))
            logger.debug('Sending free pool dict name: {}'.format(sm_mgr.shm_free_pool_dict_name))
            conn.send(bytes(sm_mgr.shm_free_pool_dict_name, "utf-8"))
            time.sleep(1)
        except KeyboardInterrupt:
            # unlink all shm blocks & sm dict
            for key in sm_mgr.shm_dict_all.keys():
                shm_temp = shared_memory.SharedMemory(key)
                shm_temp.unlink()
            
            sm_mgr.shm_free_pool_dict.shm.unlink()
            sm_mgr.shm_dict_all.shm.unlink()

            s.close()
            logger.info("SHM Manager stopped.")
            sys.exit(0)